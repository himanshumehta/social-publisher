package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/himanshumehta/social-publisher/internal/model"
	"github.com/himanshumehta/social-publisher/internal/service"
)

type PostPipelineConfig struct {
	ScanWorkers  int
	ScanInterval time.Duration
	GenWorkers   int
	GenInterval  time.Duration
	PubWorkers   int
	PubInterval  time.Duration
}

func DefaultPostPipelineConfig() PostPipelineConfig {
	return PostPipelineConfig{
		ScanWorkers:  2,
		ScanInterval: 10 * time.Second,
		GenWorkers:   2,
		GenInterval:  10 * time.Second,
		PubWorkers:   3,
		PubInterval:  15 * time.Second,
	}
}

type PostPipeline struct {
	postSvc   *service.PostService
	scanner   *service.PostScanner
	generator *service.PostGenerator
	publisher *service.PostPublisher
	config    PostPipelineConfig
	logger    *slog.Logger
}

func NewPostPipeline(
	postSvc *service.PostService,
	scanner *service.PostScanner,
	generator *service.PostGenerator,
	publisher *service.PostPublisher,
	config PostPipelineConfig,
	logger *slog.Logger,
) *PostPipeline {
	return &PostPipeline{
		postSvc:   postSvc,
		scanner:   scanner,
		generator: generator,
		publisher: publisher,
		config:    config,
		logger:    logger,
	}
}

// Start launches all pipeline worker goroutines. Blocks until ctx is cancelled.
func (p *PostPipeline) Start(ctx context.Context) {
	p.logger.Info("starting post pipeline workers",
		"scan_workers", p.config.ScanWorkers,
		"gen_workers", p.config.GenWorkers,
		"pub_workers", p.config.PubWorkers,
	)

	for i := 0; i < p.config.ScanWorkers; i++ {
		go p.scanLoop(ctx, i)
	}

	for i := 0; i < p.config.GenWorkers; i++ {
		go p.generateLoop(ctx, i)
	}

	for i := 0; i < p.config.PubWorkers; i++ {
		go p.publishLoop(ctx, i)
	}

	<-ctx.Done()
	p.logger.Info("post pipeline workers stopped")
}

func (p *PostPipeline) scanLoop(ctx context.Context, workerID int) {
	ticker := time.NewTicker(p.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.safeRun("scan", workerID, func() { p.processScan(ctx, workerID) })
		}
	}
}

func (p *PostPipeline) processScan(ctx context.Context, workerID int) {
	post, err := p.postSvc.PickForScan(ctx)
	if err != nil {
		p.logger.Error("scan worker: pick failed", "worker", workerID, "error", err)
		return
	}
	if post == nil {
		return
	}

	p.logger.Info("scan worker: processing",
		"worker", workerID,
		"post_id", post.ID,
		"url", post.SourceURL,
	)

	if post.SourceURL == "" {
		_ = p.postSvc.CompleteScan(ctx, post.ID, &model.ScannedContent{})
		return
	}

	scanned, err := p.scanner.Scan(ctx, post.SourceURL)
	if err != nil {
		p.logger.Error("scan worker: scan failed",
			"worker", workerID,
			"post_id", post.ID,
			"error", err,
		)
		_ = p.postSvc.FailScan(ctx, post.ID, err.Error())
		return
	}

	if err := p.postSvc.CompleteScan(ctx, post.ID, scanned); err != nil {
		p.logger.Error("scan worker: complete failed",
			"worker", workerID,
			"post_id", post.ID,
			"error", err,
		)
	}
}

func (p *PostPipeline) generateLoop(ctx context.Context, workerID int) {
	ticker := time.NewTicker(p.config.GenInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.safeRun("gen", workerID, func() { p.processGenerate(ctx, workerID) })
		}
	}
}

func (p *PostPipeline) processGenerate(ctx context.Context, workerID int) {
	post, err := p.postSvc.PickForGenerate(ctx)
	if err != nil {
		p.logger.Error("gen worker: pick failed", "worker", workerID, "error", err)
		return
	}
	if post == nil {
		return
	}

	p.logger.Info("gen worker: generating content",
		"worker", workerID,
		"post_id", post.ID,
		"platforms", post.TargetPlatforms,
	)

	req := &service.GenerateRequest{
		ScannedContent: post.SourceMetadata,
		Platforms:      post.TargetPlatforms,
		LinkedVideoURL: post.LinkedVideoURL,
		SourceURL:      post.SourceURL,
		CoverImageURL:  post.CoverImageURL,
		Tone:           post.Tone,
		BrandVoice:     post.BrandVoice,
	}

	if post.SourceMetadata == nil {
		req.ManualTitle = post.Title
		req.ManualContent = post.Content
	}

	result, err := p.generator.Generate(ctx, req)
	if err != nil {
		p.logger.Error("gen worker: generation failed",
			"worker", workerID,
			"post_id", post.ID,
			"error", err,
		)
		_ = p.postSvc.FailGeneration(ctx, post.ID, err.Error())
		return
	}

	if err := p.postSvc.CompleteGeneration(ctx, post.ID, result); err != nil {
		p.logger.Error("gen worker: complete failed",
			"worker", workerID,
			"post_id", post.ID,
			"error", err,
		)
		return
	}

	// Run system review inline (fast, no external API calls)
	if err := p.postSvc.CompleteSystemReview(ctx, post.ID); err != nil {
		p.logger.Error("gen worker: system review failed",
			"worker", workerID,
			"post_id", post.ID,
			"error", err,
		)
	}

	p.logger.Info("gen worker: generation complete",
		"worker", workerID,
		"post_id", post.ID,
		"tokens_used", result.TokensUsed,
	)
}

func (p *PostPipeline) publishLoop(ctx context.Context, workerID int) {
	ticker := time.NewTicker(p.config.PubInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.safeRun("pub", workerID, func() { p.processPublish(ctx, workerID) })
		}
	}
}

// safeRun wraps a worker function with panic recovery so goroutines
// don't crash silently and leave posts stuck in transient states.
func (p *PostPipeline) safeRun(stage string, workerID int, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("worker panic recovered",
				"stage", stage,
				"worker", workerID,
				"panic", r,
			)
		}
	}()
	fn()
}

func (p *PostPipeline) processPublish(ctx context.Context, workerID int) {
	post, err := p.postSvc.PickForPublish(ctx)
	if err != nil {
		p.logger.Error("pub worker: pick failed", "worker", workerID, "error", err)
		return
	}
	if post == nil {
		return
	}

	p.logger.Info("pub worker: publishing",
		"worker", workerID,
		"post_id", post.ID,
		"platforms", post.TargetPlatforms,
		"retry_count", post.RetryCount,
	)

	results, err := p.publisher.Publish(ctx, post)
	if err != nil {
		p.logger.Error("pub worker: publish failed",
			"worker", workerID,
			"post_id", post.ID,
			"error", err,
		)
		// On total failure, mark as publish_failed so it can be retried
		if failErr := p.postSvc.FailPublish(ctx, post.ID, err.Error(), nil); failErr != nil {
			p.logger.Error("pub worker: fail publish failed",
				"worker", workerID,
				"post_id", post.ID,
				"error", failErr,
			)
		}
		return
	}

	// CompletePublish handles partial success vs total failure internally
	if err := p.postSvc.CompletePublish(ctx, post.ID, results); err != nil {
		p.logger.Error("pub worker: complete failed",
			"worker", workerID,
			"post_id", post.ID,
			"error", err,
		)
		return
	}

	published := 0
	failed := 0
	for _, r := range results {
		if r.Status == "published" {
			published++
		} else {
			failed++
		}
	}

	p.logger.Info("pub worker: publish complete",
		"worker", workerID,
		"post_id", post.ID,
		"published", published,
		"failed", failed,
		"retry_count", post.RetryCount,
	)
}
