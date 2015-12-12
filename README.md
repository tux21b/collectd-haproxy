# collectd-haproxy

Command collectd-haproxy implements a [collectd](https://collectd.org/) exec
plugin to collect metrics from [haproxy](http://www.haproxy.org/) via an
admin socket.

## Installation

Simply use the `go get` tool to fetch and install the command line program:

    go get github.com/tux21b/collectd-haproxy

## Configuration

Add the following to your collectd config (e.g. `/etc/collectd/conf.d/haproxy`)
to use this plugin:

    LoadPlugin exec
    <Plugin exec>
        Exec "haproxy" "/path/to/collectd-haproxy" "-silent"
    </Plugin>

## License

> Copyright (c) 2015 by Christoph Hack <christoph@tux21b.org>
> All rights reserved. Distributed under the Simplified BSD License.
