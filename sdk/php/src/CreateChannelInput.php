<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class CreateChannelInput
{
    /** @param list<string> $tags */
    public function __construct(
        public string $id,
        public string $name,
        public array $tags,
        public string $workspaceRoot,
        public string $dataSourceId,
    ) {
    }
}
