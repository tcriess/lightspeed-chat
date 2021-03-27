FROM phusion/baseimage:focal-1.0.0-alpha1-amd64

RUN rm -f /etc/service/sshd/down
RUN /etc/my_init.d/00_regen_ssh_host_keys.sh
RUN apt-get update
RUN apt-get install -y wget build-essential sudo
RUN wget https://godeb.s3.amazonaws.com/godeb-amd64.tar.gz
RUN tar -xf godeb-amd64.tar.gz
RUN ./godeb install 1.16.2
RUN wget https://raw.githubusercontent.com/mitchellh/vagrant/master/keys/vagrant.pub
RUN cat vagrant.pub >> /root/.ssh/authorized_keys && rm -f vagrant.pub
