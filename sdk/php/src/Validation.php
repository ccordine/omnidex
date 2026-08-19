<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final class Validation
{
    private const SSL_MODES = ['disable', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full'];

    private function __construct()
    {
    }

    public static function configuration(string $baseUrl, string $token): void
    {
        self::baseUrl($baseUrl, 'Omnidex base URL');
        if (strlen($token) < 32 || strlen($token) > 4096 || preg_match('/^[\x21-\x7e]+$/D', $token) !== 1) {
            throw new \InvalidArgumentException('Omnidex integration token must contain 32..4096 exact visible ASCII bytes.');
        }
    }

    public static function directDataSource(DirectDataSourceInput $input): void
    {
        self::exactText($input->name, 'Data-source name');
        if ($input->port < 1 || $input->port > 65535) {
            throw new \InvalidArgumentException('PostgreSQL port must be between 1 and 65535.');
        }
        if (!in_array($input->sslMode, self::SSL_MODES, true)) {
            throw new \InvalidArgumentException('PostgreSQL SSL mode is unsupported.');
        }
        if ($input->useDsn) {
            self::exactText($input->dsn, 'PostgreSQL DSN');
            return;
        }
        self::exactText($input->host, 'PostgreSQL host');
        self::exactText($input->databaseName, 'PostgreSQL database');
        self::exactText($input->username, 'PostgreSQL username');
    }

    public static function delegatedDataSource(DelegatedDataSourceInput $input): void
    {
        self::exactText($input->name, 'Data-source name');
        self::baseUrl($input->authorityUrl, 'Delegated authority URL');
        if (preg_match('/^OMNIDEX_DELEGATED_AUTHORITY_[A-Z][A-Z0-9_]{0,93}_TOKEN$/D', $input->credentialEnv) !== 1) {
            throw new \InvalidArgumentException(
                'Credential environment variable is outside the dedicated namespace OMNIDEX_DELEGATED_AUTHORITY_*.',
            );
        }
    }

    public static function channel(CreateChannelInput $input): void
    {
        self::canonicalId('Channel ID', $input->id, 96);
        self::canonicalId('Data-source ID', $input->dataSourceId, 128);
        self::exactText($input->name, 'Channel name');
        self::exactText($input->workspaceRoot, 'Channel workspace root');
        if (count($input->tags) > 32) {
            throw new \InvalidArgumentException('Channel tags exceed 32 entries.');
        }
        $seen = [];
        foreach ($input->tags as $tag) {
            if (!is_string($tag) || $tag === '' || $tag !== trim($tag) || $tag !== strtolower($tag) ||
                strlen($tag) > 64 || array_key_exists($tag, $seen)) {
                throw new \InvalidArgumentException('Channel tags must be exact, lowercase, bounded, and unique.');
            }
            $seen[$tag] = true;
        }
    }

    public static function canonicalId(string $label, string $value, int $maximum): void
    {
        if (strlen($value) < 1 || strlen($value) > $maximum ||
            preg_match('/^[a-z0-9][a-z0-9_.:-]*$/D', $value) !== 1) {
            throw new \InvalidArgumentException($label . ' is not canonical.');
        }
    }

    public static function prompt(string $value): void
    {
        if (trim($value) === '' || strlen($value) > 4096 || str_contains($value, "\0") || preg_match('//u', $value) !== 1) {
            throw new \InvalidArgumentException('Prompt must contain 1..4096 exact nonblank UTF-8 bytes without NUL.');
        }
    }

    private static function baseUrl(string $value, string $label): void
    {
        if ($value === '' || $value !== trim($value) || str_ends_with($value, '/')) {
            throw new \InvalidArgumentException($label . ' must be canonical without a trailing slash.');
        }
        $parts = parse_url($value);
        if (!is_array($parts) || !in_array($parts['scheme'] ?? '', ['http', 'https'], true) ||
            ($parts['host'] ?? '') === '' || isset($parts['user']) || isset($parts['pass']) ||
            isset($parts['query']) || isset($parts['fragment'])) {
            throw new \InvalidArgumentException($label . ' must be one HTTP(S) origin or canonical base path.');
        }
    }

    private static function exactText(string $value, string $label): void
    {
        if ($value === '' || $value !== trim($value) || str_contains($value, "\0")) {
            throw new \InvalidArgumentException($label . ' must be exact nonblank text.');
        }
    }
}
