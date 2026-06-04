#!/bin/bash
# Reads DATABASE_URL from backend/.env and runs psql with it. Password never typed inline.
set -e
URL=$(grep '^DATABASE_URL=' /root/projects/ewallet/backend/.env | cut -d= -f2-)
psql "$URL" "$@"
