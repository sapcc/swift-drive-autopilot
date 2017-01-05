# logexpect

```bash
swift-drive-autopilot ./config.yaml | logexpect ./expected.log
```

This small utility is used by the unit tests to compare log output from the
swift-drive-autopilot to an expected pattern.

A log output might look like this:

```
2017/01/05 13:53:31 INFO: event received: new device found: /tmp/swift-drive-autopilot-test/loop1 -> /dev/loop2
2017/01/05 13:53:31 INFO: mounted /dev/loop2 to /run/swift-storage/c5f98bd45c6e6a89f1a52acb3b82830a
2017/01/05 13:53:31 INFO: event received: new device found: /tmp/swift-drive-autopilot-test/loop2 -> /dev/loop3
2017/01/05 13:53:31 INFO: mounted /dev/loop3 to /run/swift-storage/b5eebc4fd85ddb560a78193515a858ea
2017/01/05 13:53:31 ERROR: no swift-id file found on device /dev/loop2 (mounted at /run/swift-storage/c5f98bd45c6e6a89f1a52acb3b82830a)
2017/01/05 13:53:31 ERROR: no swift-id file found on device /dev/loop3 (mounted at /run/swift-storage/b5eebc4fd85ddb560a78193515a858ea)
2017/01/05 13:54:01 INFO: event received: scheduled consistency check
2017/01/05 13:54:01 ERROR: no swift-id file found on device /dev/loop2 (mounted at /run/swift-storage/c5f98bd45c6e6a89f1a52acb3b82830a)
2017/01/05 13:54:01 ERROR: no swift-id file found on device /dev/loop3 (mounted at /run/swift-storage/b5eebc4fd85ddb560a78193515a858ea)
```

And a pattern matching this log output may look like this:

```
INFO: event received: new device found: {{link1}} -> {{device1}}
INFO: mounted {{device1}} to /run/swift-storage/{{hash1}}
INFO: event received: new device found: {{link2}} -> {{device2}}
INFO: mounted {{device2}} to /run/swift-storage/{{hash2}}
ERROR: no swift-id file found on device {{device1}} (mounted at /run/swift-storage/{{hash1}})
ERROR: no swift-id file found on device {{device2}} (mounted at /run/swift-storage/{{hash2}})
INFO: event received: scheduled consistency check
ERROR: no swift-id file found on device {{device1}} (mounted at /run/swift-storage/{{hash1}})
ERROR: no swift-id file found on device {{device2}} (mounted at /run/swift-storage/{{hash2}})
```

So when comparing the input to the pattern, the following steps are performed:

1. The timestamps at the beginning of each log line are stripped.
2. Each occurrence of a variable like `{{foo}}` in the pattern will be compared
   to the value that it has, or (on the variable's first occurrence) it will be
   matched with the input to fill the variable.

`logexpect` exits with exit code 0 if every log line from the input matches the
corresponding line from the pattern. If not, the first mismatch will be
reported and the exit code will be 1.

If the pattern contains less lines than the standard input, `logexpect` will
exit as soon as all patterns have matched.
