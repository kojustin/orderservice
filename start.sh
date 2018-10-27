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
cname=the_server

# Install apt packages, find all missing packages and then install in a single
# batch operation.
if ! pkglist=$(apt list --installed 2>/dev/null); then
  echo "Unable to lookup installed packages"
  exit 1
fi
pkgstoinstall=()
for pkg in tmux make gcc sqlite3; do
  # Check for existence of package. The trailing "/" on the package name causes
  # a match for the exact package name.
  if ! echo "$pkglist" | grep "$pkg/" --silent; then
    pkgstoinstall+=("$pkg")
  fi
done
if [[ -n "${pkgstoinstall[*]}" ]]; then
  sudo apt install --yes "${pkgstoinstall[@]}"
fi

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
# path to the go binary. Make sure to include it if this path exists.
if [[ -f  ~/.bash_profile ]]; then
  # shellcheck source=/dev/null
  source ~/.bash_profile
fi
if ! command -v go >/dev/null; then
  if [[ ! -f /tmp/go1.11.1.linux-amd64.tar.gz ]]; then
    # Wrap cd in a shell so that the effect is localized.
    (cd /tmp && curl -O -s https://dl.google.com/go/go1.11.1.linux-amd64.tar.gz)
  fi
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
  docker stop -t 0 "$cname" > /dev/null
  docker rm "$cname" > /dev/null
fi

#
# Run the Docker image.
#

dbbasename="$(basename $dbpath)"
internalpath="/data/$dbbasename"

opts=(--env GOOGLE_MAPS_API_KEY --detach --interactive --tty)
opts+=(--mount "type=bind,source=$(pwd)/$dbpath,target=$internalpath" --rm)
opts+=(--publish 8080:8080 --name "$cname")

cmd="docker run ${opts[*]} $(cat "$target") -dbpath $internalpath"
$cmd
