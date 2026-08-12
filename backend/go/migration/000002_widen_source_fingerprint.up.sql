-- 000002: 拓宽 language_patterns.source_fingerprint 64 -> 80
-- 原因: 指纹格式为 "sha256:" + 64 位 hex = 71 字符,旧列宽 VARCHAR(64) 导致 INSERT 报
--       value too long (SQLSTATE 22001),论道首次合成直接 500。
ALTER TABLE language_patterns ALTER COLUMN source_fingerprint TYPE VARCHAR(80);

-- 历史缓存行无法区分是否由「降级提示词」生成(合成 LLM 失败时 Python 返回兜底 prompt,
-- 修复前 Go 一律以 is_valid=true 落库)。统一置失效,下次论道按当前模型配置重新合成。
UPDATE language_patterns SET is_valid = FALSE;
