#!/bin/bash
#
# Bootstraps the system, starting from a base Ubuntu 18.04 LTS OS. Installs
# development tools, builds the software, deploys the software locally.
#
# Actions are idempotent.

set -e
set -o pipefail

# Filename of the sqlite3 database file.
dbpath=artifacts/orders.db
# Name of the Docker container that runs the service.
cname=ordersvc

if [[ -z "$GOOGLE_MAPS_API_KEY" ]]; then
  echo "ERROR: GOOGLE_MAPS_API_KEY is unset, application will not work."
  exit 1
fi

# Install apt packages.
sudo apt-get update
sudo apt install --yes gcc jq make sqlite3 tmux

# Make sure NTP is running.
sudo systemctl start systemd-timesyncd

cat <<EOF > ~/.tmux.conf
# tmux configuration

# Enable mouse
set-option -g mouse on

# Set vi mode copy-paste mode
set-window-option -g mode-keys vi
bind-key -T copy-mode-vi 'v' send-keys -X begin-selection
bind-key -T copy-mode-vi 'y' send-keys -X copy-selection-and-cancel

# Colors
set -g default-terminal "screen-256color"
EOF

# If Go is not installed, install it.
#
# The PATH environment variable is updated in ~/.bash_profile to include the
# path to the go binary. Make sure to include it if this path exists. This is
# required to add go to the PATH, if it exists.
if [[ -f  ~/.bash_profile ]]; then
  # shellcheck source=/dev/null
  source ~/.bash_profile
fi
if ! command -v go >/dev/null; then
  # Download the go 1.11 package and try up to three times, make sure to check
  # the checksum.
  # shellcheck disable=SC2034
  for count in 1 2 3; do
    if [[ ! -f /tmp/go1.11.1.linux-amd64.tar.gz ]]; then
      # Wrap cd in a shell so that the effect is localized.
      (cd /tmp && curl -O -s https://dl.google.com/go/go1.11.1.linux-amd64.tar.gz)
    fi
    expectedhash=2871270d8ff0c8c69f161aaae42f9f28739855ff5c5204752a8d92a1c9f63993
    thesum=$(sha256sum /tmp/go1.11.1.linux-amd64.tar.gz | cut -f 1 -d ' ')
    echo "the sum is $thesum"
    if [[ "$thesum" != "$expectedhash" ]]; then
      echo "sha256 mismatch expected $expectedhash, got $thesum."
      rm /tmp/go1.11.1.linux-amd64.tar.gz
    else
      break
    fi
  done
  sudo tar -C /usr/local -xzf /tmp/go1.11.1.linux-amd64.tar.gz

  # Add go binary path and GOPATH/bin to PATH
  printf "export PATH=\$PATH:/usr/local/go/bin:%s/bin\\n" "$(/usr/local/go/bin/go env GOPATH)" >  ~/.bash_profile
  printf "source ~/.bashrc\\n" >> ~/.bash_profile

  # shellcheck source=/dev/null
  source ~/.bash_profile
fi

if ! command -v goimports >/dev/null; then
    go get golang.org/x/tools/cmd/goimports
fi

# Install Docker CE on Ubuntu. Based on the instructions from their website.
#   https://docs.docker.com/install/linux/docker-ce/ubuntu/#os-requirements
if ! apt list --installed 2>/dev/null | grep docker-ce --silent; then
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

# Call make to build the software and package it as a Docker image. The name of
# the output image goes into a text file.
target=artifacts/image_name.txt
make "$target"

# Create database file and populate it with inital schema.
make "$dbpath"

# If the container is running, kill it.
if docker container list --all | grep --silent "$cname"; then
  docker rm -f "$cname" > /dev/null
fi

#
# Run the Docker image.
#

dbbasename="$(basename $dbpath)"
internalpath="/data/$dbbasename"
opts=(--env GOOGLE_MAPS_API_KEY --detach --publish 8080:8080 --name "$cname")
opts+=(--mount "type=bind,source=$(pwd)/$dbpath,target=$internalpath" --rm)

cmd="docker run ${opts[*]} $(cat "$target") -dbpath $internalpath"
$cmd
