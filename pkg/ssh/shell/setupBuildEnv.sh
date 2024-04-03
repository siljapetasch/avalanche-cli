#!/usr/bin/env bash
set -e
#name:TASK [install gcc if not available]
export DEBIAN_FRONTEND=noninteractive
until sudo apt-get -y update -o DPkg::Lock::Timeout=120; do sleep 1 && echo "Try again"; done
until sudo apt-get -y install -o DPkg::Lock::Timeout=120 gcc; do sleep 1 && echo "Try again"; done
#name:TASK [install go]
go version || sudo snap install go --classic
