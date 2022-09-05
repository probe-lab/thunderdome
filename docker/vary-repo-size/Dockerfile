ARG TAG
FROM 147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:kubo-$TAG
ADD /container-init.d/* /container-init.d/
RUN chmod a+x /container-init.d/*
CMD ["daemon", "--enable-gc", "--migrate=true", "--agent-version-suffix=docker"]
