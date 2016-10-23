# norobo
Norobo is a spam phone call blocker for land lines.  It uses a modem (remember those old things?) connected to a PC, RPi, etc. and plugged into any phone jack in your house.

### Motivation
Spam / robo calling is an epidemic and relativly little has been done to stop it.  There are some great services available like Nomorobo but unfortunately not all carriers offer the simulring feature it requires.  There are also features I want that that type of service may not offer.  E.g., temporarily block a number if it calls more than once in a minute, simultaneously check multiple online blacklist services, web interface for the call log, etc.

### How it works
When a call comes in, your phone rings once, `norobod` gets the caller ID from the modem, runs the caller's info through the filters, and hangs up if the filters flag it as spam. You feel immediate pleasure from hearing the single ring, knowing `norobod` just hung up on a sleezy spammer.

### Compatibility
It has been tested on *Linux* using a *Zoom 3095 USB Mini External Modem*.  It can be run on an RPi or other small ARM based computers capable of running Linux & Go apps.  It should also (in theory) work with any Hayes compatible modem that supports caller ID.  It may also work on Windows and OS X.

### Install
Currently, it must be built from source.  You'll need the [Go tools](https://golang.org/doc/install), if you don't already have them installed.  Then clone this repo and build the code in `cmd/norobod`.

### How call filters work
`norobod` currently supports two filters: local and Twilio. 
#### Local
Local filters run locally and do not send incoming call info out to external services to perform filtering. There are two types of local filters: `allow` and `block`. Both are optional. `Allow` filters are special in that they run first and not concurrently with any other filters. If a call matches an allow filter, it is allowed through immediately with no further filtering. `Block` filters run concurrently with the Twilio filter, if configured. If a call matches a `block` filter, `norobod` will answer the call, immediately hangup, and cancel any other concurrent filters that are running.
#### Twilio
The Twilio filter is optional and requires a [Twilio](https://www.twilio.com) account. Creating an account is easy and only takes a few minutes. There is a charge for each lookup but it is minimal ($0.005 per lookup at the time of this writing). Once you've set up an account, go to the [Lookup Add-ons](https://www.twilio.com/console/lookup/add-ons) page, select the *Whitepages Pro Phone Reputation* add-on, and install it (Note: it installs on your Twilio account, not your local PC).

### Filter Configuration
#### Local
Local `allow` and `block` filters are configured using a simple CSV format.  The fields are (in order): `description`, `name`, `number`, `function`.  All fields are strings.  The `name` and `number` fields are [regular expressions](https://golang.org/pkg/regexp/syntax/).  The `function` field must be either an empty string or one of the built in filter functions:
- `NameContainsNumber` matches any call where the caller's name contains the caller's number.  All symbols are stripped from both before the comparison is made.
- `NumberContainsName` is the opposite of the previous rule.  It matches any call where the caller's number contains the caller's name.  All symbols are stripped from both before the comparison is made.
See `filter/block.csv` for examples.

The same format is used for `allow` and `block` filter files. Create a file for one or both as desired. Use the `-allow` and `-block` command line options to enable them. E.g., 
```
norobod -allow path/to/allow.csv -block path/to/block.csv <other options>
```

#### Twilio
There are no configuration files for the `Twilio` filter. To enable it, run `norobod` with the `-twlo-sid` and `-twlo-token` command line arguments. E.g.,
```
norobod <other options> -twlo-sid <your Twilio SID> -twlo-token <your Twilio token>
```
Your Twilio `Account SID` and `Auth Token` can be found by logging into your Twilio account and looking on the home tab.

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
### Running as a daemon
There is an example init script in `etc/norobo` that may work for you if you're running Linux.
- Copy `etc/norobo` from the source to `/etc/init.d/`
- Open the init script in a text editor and make sure the `cmd=` line is starting `norobod` with the command line options you want
- `sudo chmod +x /etc/init.d/norobo`
- `sudo update-rc.d norobo defaults`
- `sudo service norobo start`
- To make sure it's running, `tail /var/log/norobo.log`

### Call log
Use the `-call-log path/to/call_log.csv` command line option to specify a call log file.  The file is a comma separated value format.  The fields are (in order): `time`, `caller name`, `caller number`, `action taken`, `filter name`, `rule description`.  The call log can also be viewed by web browser on port `7080`.

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
