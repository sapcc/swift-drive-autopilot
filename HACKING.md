# Example development setup

Here's how I run the autopilot on my development box.

General note: All commands can be executed as a regular user. Usage of `sudo` indicates when super-user privileges are
required.

General note 2: I'm assuming Linux here. Good luck getting `cryptsetup` to work on macOS. (Windows might work with the WSL.)

## Initial setup

Since I don't have physical disks to spare for the purpose of testing, I use image files that are set up as loop
devices. First, create a place to store your disk images. I will assume `~/disks` here:

```bash
$ mkdir $HOME/disks
```

Create empty disk images: (This might also work with `fallocate(1)`, but I have not tried yet.)

```bash
$ cd $HOME/disks
$ for i in {1..3}; do dd if=/dev/zero bs=1M count=100 of=$i.img; done
$ ls
1.img  2.img  3.img
```

Note that no further formatting is required at this point. That's the job of the autopilot. Now prepare a configuration
file for the autopilot (as described in the [README.md](./README.md#Usage) that enables all the features that you want
to test (encryption, auto-assignment, etc.). You might assume that the following configuration would be acceptable:

```bash
$ cat $HOME/disks/config.yaml

drives:
- /home/stefan/disks/*.img
```

But this will fail. A regular file cannot be mounted (or mapped) by itself, only a device file can. Therefore, you need
to create loop devices that are backed by the image files:

```bash
$ cd $HOME/disks
$ sudo losetup --find --show 1.img
/dev/loop2
$ sudo losetup --find --show 2.img
/dev/loop3
$ sudo losetup --find --show 3.img
/dev/loop4
```

The output of `losetup --find --show` shows which device files are connected to your images. If in doubt, say `losetup` without arguments to see all configured loop devices:

```
$ losetup
NAME       SIZELIMIT OFFSET AUTOCLEAR RO BACK-FILE                                          DIO
/dev/loop1         0      0         1  0 /var/lib/docker/devicemapper/devicemapper/metadata   0
/dev/loop4         0      0         0  0 /home/stefan/disks/test3.img                         0
/dev/loop2         0      0         0  0 /home/stefan/disks/test1.img                         0
/dev/loop0         0      0         1  0 /var/lib/docker/devicemapper/devicemapper/data       0
/dev/loop3         0      0         0  0 /home/stefan/disks/test2.img                         0
```

My loop devices start counting at `2`, so the following configuration file is acceptable:

```bash
$ cat $HOME/disks/config.yaml

drives:
- /dev/loop[2-4]
```

However, I use an extra indirection step that will be useful for simulating hot swapping:

```bash
$ cd $HOME/disks
$ ln -s /dev/loop2 .
$ ln -s /dev/loop3 .
$ ln -s /dev/loop4 .
$ ls -l
total 307200
-rw-r--r-- 1 stefan users 104857600 Dez 21 12:35 1.img
-rw-r--r-- 1 stefan users 104857600 Dez 21 12:35 2.img
-rw-r--r-- 1 stefan users 104857600 Dez 21 12:35 3.img
lrwxrwxrwx 1 stefan users        10 Dez 21 12:35 loop2 -> /dev/loop2
lrwxrwxrwx 1 stefan users        10 Dez 21 12:35 loop3 -> /dev/loop3
lrwxrwxrwx 1 stefan users        10 Dez 21 12:35 loop4 -> /dev/loop4
```

Now I reference these symlinks in the autopilot configuration. The autopilot will automatically dereference these
symlinks to get to the actual device file (just like for symlinks in `/dev/disks/by-path` etc.).

```bash
$ cat $HOME/disks/config.yaml

drives:
- /home/stefan/disks/loop*
```

Now you're all setup and can launch the autopilot. Note that you should do so with `sudo`, otherwise the autopilot will
not be able to write to `/run/swift-storage` etc.:

```bash
$ cd /path/to/this/repo
$ make && sudo ./swift-drive-autopilot $HOME/disks/config.yaml
```

## After the first time

Upon reboot, all loop devices will be detached, so you must attach them again. Before starting the autopilot, take a
moment to review which devices will be seen by the autopilot! The loop device numbers may have changed:

```bash
$ sudo losetup --find --show $HOME/disks/1.img
/dev/loop3
$ sudo losetup --find --show $HOME/disks/2.img
/dev/loop4
$ sudo losetup --find --show $HOME/disks/3.img
/dev/loop5
$ ls $HOME/disks/loop*
/home/stefan/disks/loop2  /home/stefan/disks/loop3  /home/stefan/disks/loop4
$ # loop2 is not valid anymore, but loop5 is new!
$ rm $HOME/disks/loop*
$ ln -s /dev/loop5 $HOME/disks
$ ls $HOME/disks/loop*
/home/stefan/disks/loop3  /home/stefan/disks/loop4  /home/stefan/disks/loop5
$ # ready to start the autopilot now
$ cd /path/to/this/repo
$ make && sudo ./swift-drive-autopilot $HOME/disks/config.yaml
```

Now we can simulate some specific scenarios that the autopilot should be prepared to handle.

## Simulate device failure

Usually, a device failure will be detected through the kernel log, but the kernel log collector will only look for
errors on SCSI/SATA devices (`/dev/sda` etc.) and not consider loop devices. You can patch the `WatchKernelLog` function
and find a way to write stuff into the kernel log, but a much easier way to simulate a device failure is to remount the
device as read-only:

```bash
$ mount | grep srv/node
/dev/mapper/c5f98bd45c6e6a89f1a52acb3b82830a on /srv/node/foo type xfs (rw,relatime,attr2,inode64,noquota)
/dev/mapper/b5eebc4fd85ddb560a78193515a858ea on /srv/node/bar type xfs (rw,relatime,attr2,inode64,noquota)
$ sudo mount -o remount,ro /srv/node/foo
```

The simulated device failure will be detected by the next scheduled consistency check after at most 30 seconds:

```
2016/12/21 12:53:13 INFO: event received: scheduled consistency check
2016/12/21 12:53:13 ERROR: mount of /dev/mapper/c5f98bd45c6e6a89f1a52acb3b82830a at /srv/node/foo is read-only (could be due to a disk error)
2016/12/21 12:53:13 INFO: flagging /dev/loop2 as broken because of previous error
2016/12/21 12:53:13 INFO: To reinstate this drive into the cluster, delete the symlink at /run/swift-storage/broken/c5f98bd45c6e6a89f1a52acb3b82830a
2016/12/21 12:53:13 INFO: unmounted /srv/node/foo
```

## Simulate hot swap

When a drive is hot-swapped, its device file (e.g. `/dev/sda`) vanishes and then reappears when the new drive is
connected. We can simulate this by deleting the symlink to the loop device:

```bash
$ rm $HOME/disks/loop2
$ ...
$ ln -s /dev/loop2 $HOME/disks
```

Here's how the autopilot reacts according to its log:

```
2016/12/21 12:55:38 INFO: event received: device removed: /dev/loop2
2016/12/21 12:55:38 INFO: unmounted /srv/node/foo
2016/12/21 12:55:38 INFO: LUKS container /dev/mapper/c5f98bd45c6e6a89f1a52acb3b82830a closed
...
2016/12/21 12:56:08 INFO: event received: new device found: /home/stefan/disks/loop2 -> /dev/loop2
2016/12/21 12:56:11 INFO: LUKS container at /dev/loop2 opened as /dev/mapper/c5f98bd45c6e6a89f1a52acb3b82830a
2016/12/21 12:56:11 INFO: mounted /dev/mapper/c5f98bd45c6e6a89f1a52acb3b82830a to
/run/swift-storage/c5f98bd45c6e6a89f1a52acb3b82830a
2016/12/21 12:56:11 INFO: mounted /dev/mapper/c5f98bd45c6e6a89f1a52acb3b82830a to /srv/node/foo
2016/12/21 12:56:11 INFO: unmounted /run/swift-storage/c5f98bd45c6e6a89f1a52acb3b82830a
```
