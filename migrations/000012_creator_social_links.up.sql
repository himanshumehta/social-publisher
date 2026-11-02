ALTER TABLE creator_profiles ADD COLUMN IF NOT EXISTS facebook_page_url TEXT NOT NULL DEFAULT '';
ALTER TABLE creator_profiles ADD COLUMN IF NOT EXISTS twitter_handle TEXT NOT NULL DEFAULT '';
ALTER TABLE creator_profiles ADD COLUMN IF NOT EXISTS linkedin_profile_url TEXT NOT NULL DEFAULT '';
ALTER TABLE creator_profiles ADD COLUMN IF NOT EXISTS threads_handle TEXT NOT NULL DEFAULT '';
