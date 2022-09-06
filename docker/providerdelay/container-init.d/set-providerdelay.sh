if [ -z "$SEARCH_DELAY" ]; then
  echo "Need to set SEARCH_DELAY, eg 50ms"
  exit 1
fi
echo "setting Internal.Bitswap.ProviderSearchDelay to $SEARCH_DELAY"
ipfs config Internal.Bitswap.ProviderSearchDelay "$SEARCH_DELAY"
