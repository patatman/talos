<!-- markdownlint-disable -->
## talosctl gen crt

Generates an X.509 Ed25519 certificate

### Synopsis

Generates an X.509 Ed25519 certificate

```
talosctl gen crt [flags]
```

### Options

```
      --ca string     path to the PEM encoded CERTIFICATE
      --csr string    path to the PEM encoded CERTIFICATE REQUEST
  -h, --help          help for crt
      --hours int     the hours from now on which the certificate validity period ends (default 24)
      --name string   the basename of the generated file
```

### Options inherited from parent commands

```
      --context string       Context to be used in command
  -e, --endpoints strings    override default endpoints in Talos configuration
  -n, --nodes strings        target the specified nodes
      --talosconfig string   The path to the Talos configuration file (default "/home/user/.talos/config")
```

### SEE ALSO

* [talosctl gen](talosctl_gen.md)	 - Generate CAs, certificates, and private keys

