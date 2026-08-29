<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class SendMessageResult
{
    public function __construct(
        public Channel $channel,
        public ChannelMessage $userMessage,
        public Job $job,
    ) {
    }
}
