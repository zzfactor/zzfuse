#!/bin/sh
set -eu
exec docker run --rm -p 4317:4317 -p 4318:4318 -p 18888:18888 \
  --name zzfuse "${IMAGE:-zzfactor/zzfuse:dev}" "$@"
