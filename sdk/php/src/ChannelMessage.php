<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class ChannelMessage
{
    public function __construct(
        public int $id,
        public string $channelId,
        public string $role,
        public string $content,
        public string $createdAt,
    ) {
    }
}
