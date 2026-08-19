<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class Channel
{
    /** @param list<string> $tags */
    public function __construct(
        public string $id,
        public string $scope,
        public string $name,
        public array $tags,
        public int $projectId,
        public string $workspaceRoot,
        public string $dataSourceId,
        public string $mode,
    ) {
    }
}
