CREATE TABLE `package` (
  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  `name` text COLLATE NOCASE,
  `description` text,
  `keywords` JSON,
  `homepage` text,
  `version` text,
  `version_normalized` text,
  `license` JSON,
  `source_url` text,
  `source_type` text,
  `source_reference` text,
  `dist_url` text,
  `dist_type` text,
  `dist_reference` text,
  `dist_shasum` text,
  `type` text,
  `support_issues` text,
  `support_source` text,
  `time` text,
  `extra` JSON,
  `created_at` datetime,
  `updated_at` datetime,
  `funding` JSON,
  `autoload` JSON,
  `suggest` JSON
);

CREATE UNIQUE INDEX `idx_package_name_version` ON `package` (`name`, `version`);
CREATE INDEX `idx_package_name` ON `package` (`name`);

CREATE TABLE `author` (
  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  `name` text NOT NULL,
  `email` text NOT NULL,
  `homepage` text,
  `package_id` integer NOT NULL,
  CONSTRAINT `fk_author_package_id` FOREIGN KEY (`package_id`) REFERENCES `package` (`id`) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE UNIQUE INDEX `author_name_email_package_id_unique` ON `author` (`name`, `email`, `package_id`);
CREATE INDEX `author_package_id_index` ON `author` (`package_id`);

CREATE TABLE `require` (
  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  `name` text NOT NULL,
  `version` text NOT NULL,
  `package_id` integer NOT NULL,
  `created_at` datetime,
  `updated_at` datetime,
  CONSTRAINT `fk_require_package_id` FOREIGN KEY (`package_id`) REFERENCES `package` (`id`) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE UNIQUE INDEX `require_name_version_package_id_unique` ON `require` (`name`, `version`, `package_id`);
CREATE INDEX `require_package_id_index` ON `require` (`package_id`);

CREATE TABLE `require_dev` (
  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  `name` text NOT NULL,
  `version` text NOT NULL,
  `package_id` integer NOT NULL,
  `created_at` datetime,
  `updated_at` datetime,
  CONSTRAINT `fk_require_dev_package_id` FOREIGN KEY (`package_id`) REFERENCES `package` (`id`) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE UNIQUE INDEX `require_dev_name_version_package_id_unique` ON `require_dev` (`name`, `version`, `package_id`);
CREATE INDEX `require_dev_package_id_index` ON `require_dev` (`package_id`);
