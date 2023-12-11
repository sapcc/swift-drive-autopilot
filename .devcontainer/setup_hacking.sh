mkdir $HOME/disks

sudo losetup -D

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

swift-id-pools:
  - type: hdd
    prefix: swift
    start: 1
    end: 3
    spareInterval: 2
  - type: ssd
    prefix: swift
    postfix: ssd
    start: 1
    end: 3
    spareInterval: 2
  - type: nvme
    prefix: swift
    postfix: nvme
    start: 1
    end: 3
    spareInterval: 0


drives:
- /home/vscode/disks/loop*
EOF