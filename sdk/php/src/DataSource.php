<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class DataSource
{
    /** @param array<string, mixed> $fields */
    public function __construct(
        public string $id,
        public string $name,
        public string $driver,
        public string $executionMode,
        public bool $readOnly,
        public array $fields,
    ) {
    }
}
