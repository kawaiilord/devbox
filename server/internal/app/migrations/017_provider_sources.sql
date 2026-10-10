DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media_sources'::regclass
      AND conname='media_sources_source_type_check' AND pg_get_constraintdef(oid) NOT LIKE '%synology%') THEN
        ALTER TABLE media_sources DROP CONSTRAINT media_sources_source_type_check;
        ALTER TABLE media_sources ADD CONSTRAINT media_sources_source_type_check CHECK
        (source_type IN ('webdav','emby','quark','synology','qnap','fnos','nextcloud','seafile','truenas',
         'bilibili','youtube','douyin','tiktok','twitch','huya','douyu','acfun','cctv'));
    END IF;
END $$;
