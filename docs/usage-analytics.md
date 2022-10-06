# Disabling usage analytics

By default, Barbe collects anonymous usage analytics. You can disable this by setting the environment variables `DO_NOT_TRACK=1` or `BARBE_DISABLE_ANALYTICS=1`.

Take a look at the implementation of the analytics collection [here](../analytics/analytics.go).