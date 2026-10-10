CREATE TABLE IF NOT EXISTS room_features (
    room_code CHAR(6) PRIMARY KEY REFERENCES rooms(code) ON DELETE CASCADE,
    version BIGINT NOT NULL DEFAULT 0,
    data JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS room_features_visibility_idx
    ON room_features ((data->>'visibility'));

-- Preserve the active media of rooms created before playlists existed.
INSERT INTO room_features(room_code,version,data)
SELECT code,1,jsonb_build_object(
 'version',1,'visibility','private','description','','category','','tags','[]'::jsonb,
 'allow_guests',false,'roles','{}'::jsonb,'permissions','{}'::jsonb,'banned_ids','[]'::jsonb,'auto_next',true,
 'active_item_id',CASE WHEN source_url<>'' OR media_source_id IS NOT NULL THEN 'initial-item' ELSE '' END,
 'playlist',CASE WHEN source_url<>'' OR media_source_id IS NOT NULL THEN jsonb_build_array(jsonb_build_object(
   'id','initial-item','title',name,'episode',episode,'selected_source_id','initial-source',
   'sources',jsonb_build_array(jsonb_build_object('id','initial-source','label','原始片源','owner_id',owner_id,
    'url',source_url,'media_source_id',COALESCE(media_source_id,''),'media_path',COALESCE(media_path,'')))))
   ELSE '[]'::jsonb END)
FROM rooms ON CONFLICT(room_code) DO NOTHING;
