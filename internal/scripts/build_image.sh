#!/bin/bash

set -euo pipefail

usage() {
	cat <<EOF
Usage: ${0##*/} [-a ANNOTATIONS_FILE] [-c CHECKPOINT_PATH] [-i IMAGE_NAME]
Create OCI image from a checkpoint tar file.

    -a          path to the annotations file
    -c          path to the checkpoint file
    -i          name of the resulting image
EOF
	exit 1
}

annotationsFilePath=""
checkpointPath=""
imageName=""

while getopts ":a:c:i:" opt; do
	case ${opt} in
	a)
		annotationsFilePath=$OPTARG
		;;
	c)
		checkpointPath=$OPTARG
		;;
	i)
		imageName=$OPTARG
		;;
	:)
		echo "Option -$OPTARG requires an argument."
		usage
		;;
	\?)
		echo "Invalid option: -$OPTARG"
		usage
		;;
	esac
done
shift $((OPTIND - 1))

if [[ -z $annotationsFilePath || -z $checkpointPath || -z $imageName ]]; then
	echo "All options (-a, -c, -i) are required."
	usage
fi

if ! command -v buildah &>/dev/null; then
	echo "buildah is not installed. Please install buildah before running 'checkpointctl build' command."
	exit 1
fi

if [[ ! -f $annotationsFilePath ]]; then
	echo "Annotations file not found: $annotationsFilePath"
	exit 1
fi

if [[ ! -f $checkpointPath ]]; then
	echo "Checkpoint file not found: $checkpointPath"
	exit 1
fi

newcontainer=$(buildah from scratch)

buildah add "$newcontainer" "$checkpointPath"

while IFS= read -r line; do
	key=$(echo "$line" | cut -d '=' -f 1)
	value=$(echo "$line" | cut -d '=' -f 2-)
	buildah config --annotation "$key=$value" "$newcontainer"
done <"$annotationsFilePath"

buildah commit "$newcontainer" "$imageName"

buildah rm "$newcontainer"

echo "Checkpoint image created successfully: $imageName"
