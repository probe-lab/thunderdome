ARG FROM_IMAGE_NAME
FROM "$FROM_IMAGE_NAME"

# Create the init scripts directory (needed for pre 0.13 releases)
RUN mkdir -p /container-init.d \
  && chown ipfs:users /container-init.d

# Copy the init stuff 
ADD container_daemon /usr/local/bin/start_ipfs
ADD container_init_run /usr/local/bin/container_init_run
RUN chmod 0755 /usr/local/bin/start_ipfs
RUN chmod 0555 /usr/local/bin/container_init_run

# Use the init stuff to set appropriate config to run a gateway
ADD /container-init.d/* /container-init.d/
RUN chmod a+x /container-init.d/*

EXPOSE 8080
CMD ["daemon", "--enable-gc", "--migrate=true", "--agent-version-suffix=docker"]
