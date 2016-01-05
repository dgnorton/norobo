# norobo
Norobo is a spam phone call blocker for land lines.  It uses a modem (remember those old things?) connected to a PC, RPi, etc. and plugged into any phone jack in your house.

### Motivation
Spam / robo calling is an epidemic and relativly little has been done to stop it.  There are some great services available like Nomorobo but unfortunately not all carriers offer the simulring feature it requires.  There are also features I want that that type of service may not offer.  E.g., temporarily block a number if it calls more than once in a minute, simultaneously check multiple online blacklist services, web interface for the call log, etc.

### How it works
When a call comes in, your phone rings once, norobo gets the caller ID from the modem, checks a list of blocked numbers / rules, and hangs up on them if it matches the list.

### Compatibility
It has been tested on *Linux* using a *Zoom 3095 USB Mini External Modem*.  It can be run on an RPi or other small ARM based computers capable of running Linux & Go apps.  It should also (in theory) work with any Hayes compatible modem that supports caller ID.  It may also work on Windows and OS X.

### Install
Currently, it must be built from source.  You'll need the [Go tools](https://golang.org/doc/install), if you don't already have them installed.  Then clone this repo and build the code in `cmd/norobod`.

### Configuration
Call blocking rules are loaded from `block.csv` in the same directory as `norobod` is running.  `block.csv` is a comma separated value format.  The fields are (in order): description, name, number, function.  All fields are strings.  The `name` and `number` fields are [regular expressions](https://golang.org/pkg/regexp/syntax/).  The `function` field must be one of the built in filter functions:
- `NameContainsNumber` matches any call where the caller's name contains the caller's number.  All symbols are stripped from both before the comparison is made.
- `NumberContainsName` is the opposite of the previous rule.  It matches any call where the caller's number contains the caller's name.  All symbols are stripped from both before the comparison is made.

### Running
On Linux:
- Plug the modem in to the PC
- `ls /dev` to find the modem's name. It's `/dev/ttyACM0` for me.
- `./norobod -c "/dev/ttyACM0,19200,n,8,1"`

If it exits immediately with `permission denied`, you probably need to add yourself to the `dialout` group.  Check who owns the modem and what group it belongs to first:
```
$ ls -l /dev/ttyACM0
crw-rw---- 1 root dialout 166, 0 Dec 30 21:52 /dev/ttyACM0
```
Then confirm our suspicion that your user is not in the modem's group:
```
$ groups dgnorton
dgnorton : dgnorton adm cdrom sudo
```
Then add yourself to the `dialout` group:
```
$ usermod -a -G dialout dgnorton
```
### Call log
Calls are logged to `call_log.csv` in the same directory that `norobod` is running.  The file is a comma separated value format.  The fields are (in order): time, caller name, caller number, action taken, filter name, rule description.

### Desktop notifications
If you happen to be running it on a Ubuntu Desktop machine and would like desktop notifications, this shell script one-liner will work.
```
while inotifywait -e close_write call_log.csv; do notify-send "Call From..." "`tail -n1 call_log.csv | awk -F, '{ print $2 " at " $3 }'`"; done
```

### Dev notes
I use `socat` to create a pair of connected virtual serial ports for development and debug.  There's a hacky little modem simulator in `cmd/modem`.  `norobod` will talk to it and think it's talking to a modem.  The modem simulator has a simple HTTP API for simulating inbound calls.
Create a pair of connected virtual serial ports:
```
$ socat -d -d pty,raw,echo=0 pty,raw,echo=0
2015/12/30 20:22:20 socat[9498] N PTY is /dev/pts/30
2015/12/30 20:22:20 socat[9498] N PTY is /dev/pts/31
```
It will tell you the ports it created.  In the example above it's `/dev/pts/30` and `/dev/pts/31`.  Pass one of those to the connect string when starting the modem simulator and the other in the connect string to `norobod`.

Simulate a call
```
curl -X POST http://localhost:8087/call --data "name=John+Doe&number=111-222-3333"
```
