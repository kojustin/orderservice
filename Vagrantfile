# Vagrantfile installs Ubuntu 18.04 LTS
#
# See https://app.vagrantup.com/ubuntu/boxes/bionic64
Vagrant.configure("2") do |config|
  config.vm.box = "ubuntu/bionic64"
  config.vm.network "forwarded_port", guest: 8080, host: 8080
end
