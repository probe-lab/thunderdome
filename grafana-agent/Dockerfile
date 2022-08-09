FROM grafana/agent:v0.26.1

COPY agent.yaml /etc/agent/agent.yaml

CMD [ "-metrics.wal-directory=/data/grafana-agent", "-config.expand-env", "-config.file=/etc/agent/agent.yaml" ]