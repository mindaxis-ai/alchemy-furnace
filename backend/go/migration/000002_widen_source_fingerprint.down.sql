-- 000002 down: 还原列宽。若已有 71 字符指纹数据,此 ALTER 会报错,属预期(数据不兼容旧 schema)。
ALTER TABLE language_patterns ALTER COLUMN source_fingerprint TYPE VARCHAR(64);
