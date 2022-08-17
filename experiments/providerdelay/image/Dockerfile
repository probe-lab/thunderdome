ARG kuboimage
FROM $kuboimage

ADD /container-init.d/* /container-init.d/
ARG config
ADD /configs/$config /container-init.d/
RUN chmod a+x /container-init.d/*

EXPOSE 8080
