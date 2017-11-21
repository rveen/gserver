# gserver, in Go

gserver is an experimental web server adapted to serve OGDL templates along with
static content. It is not optimized and not well tested, as of now.

## Features

 - The file extension of (some) files below the document root is optional
 - Trailing slash and index files detection
 - Markdown rendering on the fly.
 - Login, Logout example functionality
 - TODO: Uploaded files go to files/user/*
 - Template functionality extensible through Go plugins

## Parameter substitution in paths

While resolving a path in the document root, entries of the form _token are
used for path elements not directly found in the file system. In that case 'token'
will be available later as a variable, set to the unknown path element.

For example:

     /john/blog/1

will be sent to

    /_user/blog/_id/index.htm

if that path is present. Two variables will be available in the context:

    user=john
    id=1

## Routes

There are two routes configured in gserver:

    /:user/file/*filepath
	/*filepath

The first one goes to a static file handler which will handle files from a
root configured in .conf/config.g (TODO).

The second handles the document root with its possible parameter substitution
mechanism explained above. Any path that has not the form /token/file/* goest
to this second route.


## Templates

## Remote functions

OGDL remote functions (RPC servers) can be configured in .conf/config:

    ogdlrf
      git
        host localhost:1135
      zoekt
        host localhost:1166

TODO: document in ogdl-go how to create servers and clients.

## Plugins

Plugins that are present in .conf/plugin are loaded in made available in the
request context. Public methods in those plugins can then be accessed in templates.
If a plugin database.so is present, a Database object is expected which will be
placed in the context so that $database.Method() can be called in the template.




