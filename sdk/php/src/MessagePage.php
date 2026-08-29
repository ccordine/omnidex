<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class MessagePage
{
    /** @param list<ChannelMessage> $messages */
    public function __construct(
        public string $channelId,
        public array $messages,
        public ?int $nextBeforeId,
        public bool $hasMore,
    ) {
    }
}
