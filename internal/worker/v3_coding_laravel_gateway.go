package worker

func laravelComposeFile(profile directCodingProjectVersionProfile, hasState bool) (string, error) {
	postgresImage, err := directCodingVersionComponent(profile, "postgres_image")
	if err != nil {
		return "", err
	}
	databaseService := ""
	applicationDependency := ""
	applicationDatabaseEnvironment := ""
	applicationCommand := `["php-fpm"]`
	volumes := ""
	if hasState {
		databaseService = `  db:
    image: ` + postgresImage + `
    restart: unless-stopped
    environment:
      POSTGRES_DB: application
      POSTGRES_USER: application
      POSTGRES_PASSWORD: "${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U application -d application"]
      interval: 1s
      timeout: 3s
      retries: 30
    volumes:
      - service-state:/var/lib/postgresql
`
		applicationDependency = `    depends_on:
      db:
        condition: service_healthy
`
		applicationDatabaseEnvironment = `      DB_CONNECTION: pgsql
      DB_HOST: db
      DB_PORT: "5432"
      DB_DATABASE: application
      DB_USERNAME: application
      DB_PASSWORD: "${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"
`
		applicationCommand = `["sh", "-c", "php artisan migrate --force && exec php-fpm"]`
		volumes = `volumes:
  service-state:
`
	}
	return `services:
` + databaseService + `  app:
    build:
      context: .
      target: application
    restart: unless-stopped
` + applicationDependency + `    environment:
      APP_ENV: production
      APP_DEBUG: "false"
      APP_URL: http://localhost
      APP_KEY: "${APP_KEY:?APP_KEY is required}"
` + applicationDatabaseEnvironment + `    command: ` + applicationCommand + `
    expose:
      - "9000"
    healthcheck:
      test: ["CMD", "php", "-r", "$$s=fsockopen('127.0.0.1',9000,$$e,$$m,1); if(!$$s){exit(1);} fclose($$s);"]
      interval: 1s
      timeout: 3s
      retries: 30
  nginx:
    build:
      context: .
      target: gateway
    restart: unless-stopped
    depends_on:
      app:
        condition: service_healthy
    ports:
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:${HOST_HTTP_PORT:-0}:80"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
` + laravelNginxHealthcheck() + volumes, nil
}

func laravelNginxHealthcheck() string {
	return `    healthcheck:
      test: ["CMD", "curl", "--fail", "--silent", "--show-error", "--max-time", "2", "--output", "/dev/null", "http://127.0.0.1` + directCodingDeploymentReadinessPath + `"]
      interval: 1s
      timeout: 3s
      retries: 30
`
}

func laravelNginxConfig() string {
	return `events {}

http {
  include /etc/nginx/mime.types;
  default_type application/octet-stream;

  server {
    listen 80;
    server_name _;
    root /app/public;
    index index.php;

    location / {
      try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
      try_files $uri =404;
      include fastcgi_params;
      fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
      fastcgi_param HTTP_PROXY "";
      fastcgi_pass app:9000;
    }
  }
}
`
}
