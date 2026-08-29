<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class Job
{
    public function __construct(
        public int $id,
        public string $instruction,
        public string $pipeline,
        public string $status,
        public string $result,
        public string $error,
        public int $currentGeneration,
    ) {
    }
}
