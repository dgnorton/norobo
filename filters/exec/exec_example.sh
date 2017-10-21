#!/bin/bash


set -e

OPTIND=1

while getopts "n:" opt; do
	case "$opt" in
		n)
			number="$OPTARG"
			;;
	esac
done

shift $((OPTIND-1))

if [ "$number" -eq "1234567890" ]; then
	printf "block\n"
else
	printf "allow\n"
fi
