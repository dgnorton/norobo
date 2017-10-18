#!/bin/bash

set -e

socatPID=0
tmpdir="/tmp/norobo"
session="norobo"
twilioSID=
twilioTok=

# Parse command line arguments.
while :; do
	case "$1" in
		-k|--kill)
			# Kill socat if it's running and exit this script.
			socatPID="$(pidof socat)"
			if [ "$socatPID" -gt 1 ]; then
				kill -s TERM $socatPID
			fi
			exit 0
			;;
		--twilio)
			# Parse Twilio SID and token.
			if [ "$2" ]; then
				twilioSID=$(echo $2 | cut -d: -f1)
				twilioTok=$(echo $2 | cut -d: -f2)

				if [ -z "$twilioSID" -o -z "$twilioTok" ]; then
					echo "--twilio requires SID:Token"
					exit 1
				fi
			else
				echo "--twilio requires SID:Token"
				exit 1
			fi
			shift
			;;
		*)
			break
	esac

	shift
done

# Make sure socat is installed.
if [ -z "$(which socat)" ]; then
	echo "error: socat must be installed for this script to run"
	exit 1
fi

# Delete old tmp work dir if it exists.
if [ -d "$tmpdir" ]; then
	rm -rf $tmpdir
fi

# Create working dir.
mkdir $tmpdir

# Copy binaries and config files to working dir.
cp $GOPATH/bin/modem $tmpdir
cp $GOPATH/bin/norobod $tmpdir
#cp -r /home/dgnorton/go/src/github.com/dgnorton/norobo/cmd/norobod/web $tmpdir
cp $GOPATH/src/github.com/dgnorton/norobo/filter/block.csv $tmpdir
cp $GOPATH/src/github.com/dgnorton/norobo/filter/exec_example.sh $tmpdir

# Change to the tmp working dir.
cd $tmpdir

# Setup tmux session with three panes: 1 for modem sim, 1 for norobod, and 1 for user interaction.
tmux new-session -d -s $session
tmux split-window -h -t $session -p 50
tmux select-pane -t $session:0.0
tmux split-window -v -t $session -p 25

# Create pair of linked TTYs / serial ports.
tmux send-keys -t $session:0.0 "socat -d -d pty,raw,echo=0 pty,raw,echo=0 > $tmpdir/socat.out 2>&1 &" C-m
sleep 1

# Parse TTY names from socat's output.
modemTTY="$(cat $tmpdir/socat.out | head -1 | awk '{print $NF }')"
norobodTTY="$(cat $tmpdir/socat.out | head -2 | tail -1 | awk '{print $NF }')"

# Start the modem simulator and wait for it to initialize.
tmux send-keys -t $session:0.0 "$tmpdir/modem -c \"$modemTTY,19200,n,8,1\"" C-m
sleep 1

# Start norobod so that it uses the modem simulator.
if [ ! -z "$twilioSID" ]; then
	twilioOpts="-twlo-sid $twilioSID -twlo-token $twilioTok"
fi

tmux send-keys -t $session:0.2 "$tmpdir/norobod -c \"$norobodTTY,19200,n,8,1\" -block $tmpdir/block.csv -call-log $tmpdir/call.log -exec $tmpdir/exec_example.sh $twilioOpts" C-m

# Set the active tmux pane to the one for user interaction.
tmux select-pane -t $session:0.1
tmux send-keys -t $session:0.1 "curl -X POST \"localhost:8087/call?name=JohnDoe&number=1234567890\""

# Attach to the newly configured tmux session.
tmux -2 attach-session -t $session
