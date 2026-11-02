package model

import (
	"time"

	"github.com/google/uuid"
)

// BrandKit represents a creator's reusable brand identity.
type BrandKit struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	Name             string    `json:"name"`
	LogoURL          string    `json:"logo_url,omitempty"`
	PrimaryColor     string    `json:"primary_color,omitempty"`
	SecondaryColor   string    `json:"secondary_color,omitempty"`
	AccentColor      string    `json:"accent_color,omitempty"`
	FontHeading      string    `json:"font_heading,omitempty"`
	FontBody         string    `json:"font_body,omitempty"`
	Tone             string    `json:"tone"`
	VoiceDescription string    `json:"voice_description,omitempty"`
	Tagline          string    `json:"tagline,omitempty"`
	Hashtags         []string  `json:"hashtags"`
	CTADefault       string    `json:"cta_default,omitempty"`
	IsDefault        bool      `json:"is_default"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ContentTemplate is a parameterized text skeleton scoped to a brand kit + platform.
type ContentTemplate struct {
	ID            uuid.UUID `json:"id"`
	BrandKitID    uuid.UUID `json:"brand_kit_id"`
	UserID        uuid.UUID `json:"user_id"`
	Name          string    `json:"name"`
	Platform      string    `json:"platform"`
	TemplateBody  string    `json:"template_body"`
	Variables     []string  `json:"variables"`
	ExampleOutput string    `json:"example_output,omitempty"`
	Category      string    `json:"category"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateBrandKitRequest is the input for creating a new brand kit.
type CreateBrandKitRequest struct {
	Name             string   `json:"name"`
	LogoURL          string   `json:"logo_url,omitempty"`
	PrimaryColor     string   `json:"primary_color,omitempty"`
	SecondaryColor   string   `json:"secondary_color,omitempty"`
	AccentColor      string   `json:"accent_color,omitempty"`
	FontHeading      string   `json:"font_heading,omitempty"`
	FontBody         string   `json:"font_body,omitempty"`
	Tone             string   `json:"tone,omitempty"`
	VoiceDescription string   `json:"voice_description,omitempty"`
	Tagline          string   `json:"tagline,omitempty"`
	Hashtags         []string `json:"hashtags,omitempty"`
	CTADefault       string   `json:"cta_default,omitempty"`
}

// UpdateBrandKitRequest uses pointer fields so callers can omit unchanged values.
type UpdateBrandKitRequest struct {
	Name             *string   `json:"name,omitempty"`
	LogoURL          *string   `json:"logo_url,omitempty"`
	PrimaryColor     *string   `json:"primary_color,omitempty"`
	SecondaryColor   *string   `json:"secondary_color,omitempty"`
	AccentColor      *string   `json:"accent_color,omitempty"`
	FontHeading      *string   `json:"font_heading,omitempty"`
	FontBody         *string   `json:"font_body,omitempty"`
	Tone             *string   `json:"tone,omitempty"`
	VoiceDescription *string   `json:"voice_description,omitempty"`
	Tagline          *string   `json:"tagline,omitempty"`
	Hashtags         *[]string `json:"hashtags,omitempty"`
	CTADefault       *string   `json:"cta_default,omitempty"`
}

// CreateTemplateRequest is the input for creating a content template.
type CreateTemplateRequest struct {
	BrandKitID    uuid.UUID `json:"brand_kit_id"`
	Name          string    `json:"name"`
	Platform      string    `json:"platform"`
	TemplateBody  string    `json:"template_body"`
	Variables     []string  `json:"variables,omitempty"`
	ExampleOutput string    `json:"example_output,omitempty"`
	Category      string    `json:"category,omitempty"`
}

// RenderTemplateRequest supplies variable values for rendering.
type RenderTemplateRequest struct {
	TemplateID uuid.UUID         `json:"template_id"`
	Variables  map[string]string `json:"variables"`
}

// RenderedTemplate is the output of a render operation.
type RenderedTemplate struct {
	Platform     string `json:"platform"`
	RenderedText string `json:"rendered_text"`
	CharCount    int    `json:"char_count"`
	CharLimit    int    `json:"char_limit"`
	IsOverLimit  bool   `json:"is_over_limit"`
}
