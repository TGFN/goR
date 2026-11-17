# Vercel Deployment Support

This branch adapts the original Glitch Effects API for deployment on Vercel.

## Changes

- Detects `VERCEL=1` environment variable.
- Uses `/tmp/cache` and `/tmp/fonts` on Vercel (filesystem is read-only except `/tmp`).
- File cache is best-effort; failures are non-fatal.
- Skips automatic emoji font download on Vercel (ephemeral environment).
- Added `vercel.json` with `"framework": "go"` so Vercel treats it as a Go HTTP server.

## Deploy

1. Connect the repository (or this branch) to a Vercel project.
2. Ensure the framework is set to **Go** (or let zero-config detection work).
3. Deploy. The server listens on the `PORT` provided by Vercel.

The application already uses the standard `net/http` server and respects the `PORT` environment variable, which matches Vercel's Go runtime requirements.
