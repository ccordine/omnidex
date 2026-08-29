<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final class Authority
{
    private function __construct()
    {
    }

    public static function createDelegatedId(): string
    {
        return 'dba_' . bin2hex(random_bytes(32));
    }

    public static function validateDelegatedId(string $value): void
    {
        if (preg_match('/^dba_[0-9a-f]{64}$/D', $value) !== 1) {
            throw new \InvalidArgumentException('Delegated authority must be one opaque dba_ identity.');
        }
    }
}
