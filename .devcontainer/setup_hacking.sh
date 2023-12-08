mkdir $HOME/disks

cd $HOME/disks
for i in $(seq 1 3); do dd if=/dev/zero bs=1M count=100 of=$i.img; done

sudo losetup --find --show 1.img
sudo losetup --find --show 2.img
sudo losetup --find --show 3.img

ln -s /dev/loop1 .
ln -s /dev/loop2 .
ln -s /dev/loop3 .

cat << EOF > config.yaml
chown:
  user: "0"
  group: "0"

swift-id-pool:
 - swift-hdd-01
 - spare
 - swift-ssd-01
 - swift-nvme-01

swift-id-pools:
  - type: hdd
    prefix: swift
    postfix: hdd
    start: 1
    end: 3
    spareInterval: 20
  - type: ssd
    prefix: swift
    postfix: ssd
    start: 1
    end: 3
    spareInterval: 20
  - type: nvme
    prefix: swift
    postfix: nvme
    start: 1
    end: 3
    spareInterval: 0


drives:
- /home/vscode/disks/loop*
EOF