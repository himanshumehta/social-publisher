-- Add publish_failed status to post_status enum for retry support
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'publish_failed';
