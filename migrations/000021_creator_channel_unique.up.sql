-- Enforce one creator channel per platform per user.
CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_user_platform_unique
    ON channels(user_id, platform)
    WHERE channel_type = 'creator' AND user_id IS NOT NULL;
