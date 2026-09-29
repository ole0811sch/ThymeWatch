#!/bin/sh
curl -X POST "http://localhost:8091/alarm?expirytime=$(expr \( "$(date +%s)" + "$1" \* 60 \) \* 1000)"