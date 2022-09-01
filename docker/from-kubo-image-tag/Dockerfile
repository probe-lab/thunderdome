ARG TAG
FROM ipfs/kubo:$TAG
ADD /container-init.d/* /container-init.d/
RUN chmod a+x /container-init.d/*

EXPOSE 8080