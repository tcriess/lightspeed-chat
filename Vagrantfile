Vagrant.configure("2") do |config|
    config.vm.provider "docker" do |d|
        d.build_dir = "./vagrant/"
        d.has_ssh = true
        d.remains_running = true
    end
    config.ssh.insert_key = false
    config.ssh.username = "root"
end