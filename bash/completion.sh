#!/usr/bin/env bash

if declare -F _docker >/dev/null; then
  complete -o default -F _docker dcolor
elif declare -F __start_docker >/dev/null; then
  complete -o default -F __start_docker dcolor
fi