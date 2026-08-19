<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class DirectDataSourceInput
{
    public function __construct(
        public string $name,
        public string $host,
        public int $port,
        public string $databaseName,
        public string $username,
        public string $sslMode,
        public bool $useDsn = false,
        public string $password = '',
        public string $dsn = '',
    ) {
    }
}
