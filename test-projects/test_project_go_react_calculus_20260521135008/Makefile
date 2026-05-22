.PHONY: test build start-backend start-frontend smoke

test:
	cd backend/calculus-api && go test ./...
	cd frontend/calculus-frontend && npm test
	node scripts/smoke-test.js

build:
	cd frontend/calculus-frontend && npm run build

start-backend:
	cd backend/calculus-api && go run .

start-frontend:
	cd frontend/calculus-frontend && npm start

smoke:
	node scripts/smoke-test.js
