package worker

func laravelSkeletonMarker(profile directCodingProjectVersionProfile) (string, error) {
	version, err := directCodingVersionComponent(profile, "laravel_skeleton")
	if err != nil {
		return "", err
	}
	return "// Bootstrap profile derived from laravel/laravel v" + version + ".", nil
}

func laravelArtisanSource() string {
	return `#!/usr/bin/env php
<?php

use Illuminate\Foundation\Application;
use Symfony\Component\Console\Input\ArgvInput;

define('LARAVEL_START', microtime(true));

require __DIR__.'/vendor/autoload.php';

/** @var Application $app */
$app = require_once __DIR__.'/bootstrap/app.php';

$status = $app->handleCommand(new ArgvInput());

exit($status);
`
}

func laravelBootstrapSource(profile directCodingProjectVersionProfile) (string, error) {
	marker, err := laravelSkeletonMarker(profile)
	if err != nil {
		return "", err
	}
	return `<?php

use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Exceptions;
use Illuminate\Support\Facades\Route;
use Symfony\Component\HttpKernel\Exception\MethodNotAllowedHttpException;
use Symfony\Component\HttpKernel\Exception\NotFoundHttpException;

` + marker + `
return Application::configure(basePath: dirname(__DIR__))
    ->withRouting(using: static function (): void {
        Route::group([], base_path('routes/web.php'));
    })
    ->withExceptions(function (Exceptions $exceptions): void {
        $exceptions->render(function (MethodNotAllowedHttpException $failure) {
            return response('HTTP method is not allowed for this endpoint.', 405)
                ->header('Content-Type', 'text/plain');
        });
        $exceptions->render(function (NotFoundHttpException $failure) {
            return response('Endpoint not found.', 404)->header('Content-Type', 'text/plain');
        });
    })->create();
`, nil
}

func laravelPublicIndexSource() string {
	return `<?php

use Illuminate\Foundation\Application;
use Illuminate\Http\Request;

define('LARAVEL_START', microtime(true));

require __DIR__.'/../vendor/autoload.php';

/** @var Application $app */
$app = require_once __DIR__.'/../bootstrap/app.php';

$app->handleRequest(Request::capture());
`
}

func laravelAppConfiguration() string {
	return `<?php

return [
    'name' => 'Generated Application',
    'env' => env('APP_ENV'),
    'debug' => (bool) env('APP_DEBUG'),
    'url' => env('APP_URL'),
    'timezone' => 'UTC',
    'locale' => 'en',
    'fallback_locale' => 'en',
    'cipher' => 'AES-256-CBC',
    'key' => env('APP_KEY'),
];
`
}

func laravelDatabaseConfiguration() string {
	return `<?php

return [
    'default' => env('DB_CONNECTION'),
    'connections' => [
        'pgsql' => [
            'driver' => 'pgsql',
            'host' => env('DB_HOST'),
            'port' => env('DB_PORT'),
            'database' => env('DB_DATABASE'),
            'username' => env('DB_USERNAME'),
            'password' => env('DB_PASSWORD'),
            'charset' => 'utf8',
            'prefix' => '',
            'prefix_indexes' => true,
            'search_path' => 'public',
            'sslmode' => 'prefer',
        ],
    ],
    'migrations' => ['table' => 'migrations', 'update_date_on_publish' => true],
];
`
}
