#!/bin/sh
set -eu

: "${IMAGE_PATH:?set IMAGE_PATH to a JPEG or PNG}"

curl --fail-with-body --request POST http://localhost:8080/submissions \
  --form 'submission_id=creator-demo-001' \
  --form 'caption=Morning trail above the lake' \
  --form "image=@${IMAGE_PATH}"
