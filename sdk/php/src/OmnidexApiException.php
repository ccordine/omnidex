<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final class OmnidexApiException extends \RuntimeException
{
    public function __construct(
        public readonly int $status,
        public readonly string $apiMessage,
    ) {
        parent::__construct(sprintf(
            'Omnidex integration API failed with HTTP %d: %s',
            $status,
            $apiMessage,
        ));
    }
}
