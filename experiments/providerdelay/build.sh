#!/bin/bash
set -euo pipefail
# set -x


CONFIGS="image/configs/*.sh"

aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 147263665150.dkr.ecr.eu-west-1.amazonaws.com

for cf in $CONFIGS
do
	if [ -f "$cf" ]
 	then
 		file=${cf##*/}
 		name=${file%.sh}
 		# Build and push image
 		echo Building thunderdome:$name
		echo docker build  --build-arg config=$file -t thunderdome:$name image/
		echo docker tag thunderdome:$name 147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:$name
		echo docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:$name
	else
    	echo "\"$cf\" was not a file"
    	exit
  	fi

done
