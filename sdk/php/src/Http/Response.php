<?php

declare(strict_types=1);

namespace Omnidex\Integration\Http;

final readonly class Response
{
    /** @param array<string, string> $headers */
    public function __construct(
        public int $status,
        public array $headers,
        public string $body,
    ) {
        if ($status < 100 || $status > 599) {
            throw new \InvalidArgumentException('HTTP response status is invalid.');
        }
    }

    public function header(string $name): string
    {
        return $this->headers[strtolower($name)] ?? '';
    }
}
