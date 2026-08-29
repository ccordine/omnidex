<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class JobDetails
{
    /** @param list<array<string, mixed>> $steps @param list<array<string, mixed>> $contexts */
    public function __construct(
        public Job $job,
        public array $steps,
        public array $contexts,
    ) {
    }
}
