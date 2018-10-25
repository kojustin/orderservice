#!/bin/bash
#
# Bootstraps the system, starting from a base Ubuntu 18.04 LTS OS. Installs
# development tools, builds the software, deploys the software locally.

set -e
set -o pipefail

#
# Boostrap the development environment. Install go, tmux, docker-ce.
#

if ! apt list --installed 2>/dev/null | grep golang --silent; then
  sudo apt install --yes golang
fi
if ! apt list --installed 2>/dev/null | grep tmux --silent; then
  sudo apt install --yes tmux
fi

# Install Docker CE on Ubuntu (https://docs.docker.com/install/linux/docker-ce/ubuntu/#os-requirements)
# Check to see if Docker CE is already installed first.
if apt list --installed 2>/dev/null | grep docker-asdf --silent; then
  echo "Performing Docker CE install"
  sudo apt-get update
  sudo apt-get install --yes \
      apt-transport-https \
      ca-certificates \
      curl \
      software-properties-common
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
  sudo add-apt-repository \
     "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
     $(lsb_release -cs) \
     stable"
  sudo apt-get update
  sudo apt-get install --yes docker-ce
fi

# Make docker usable.
sudo chmod 777 /var/run/docker.sock

# Call make to build the sofware and package it as a Docker image. The name of
# the output image goes into a text file.
make artifacts/image_name.txt --silent

# Run the built image.
cmd="docker run -it --rm $(cat artifacts/image_name.txt)"
$cmd
