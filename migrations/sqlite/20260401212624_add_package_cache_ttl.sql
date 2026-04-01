CREATE TABLE `package_cache_ttl` (
  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  `package_name` text NOT NULL COLLATE NOCASE,
  `last_fetched_at` datetime
);

CREATE UNIQUE INDEX `idx_package_cache_ttl_package_name` ON `package_cache_ttl` (`package_name`);
