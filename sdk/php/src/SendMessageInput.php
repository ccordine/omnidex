<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final readonly class SendMessageInput
{
    public function __construct(
        public string $prompt,
        public ?string $delegatedDataAuthorityId = null,
    ) {
    }
}
