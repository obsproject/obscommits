<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>OBScommits</title>
    <link rel="stylesheet" href="//netdna.bootstrapcdn.com/bootstrap/3.1.1/css/bootstrap.min.css">

    <script src="//ajax.googleapis.com/ajax/libs/jquery/1.11.1/jquery.min.js"></script>
    <script src="//netdna.bootstrapcdn.com/bootstrap/3.1.1/js/bootstrap.min.js"></script>
    <script src="//cdn.jsdelivr.net/headroomjs/0.5.0/headroom.min.js"></script>

    <!-- HTML5 shim and Respond.js IE8 support of HTML5 elements and media queries -->
    <!--[if lt IE 9]>
      <script src="//oss.maxcdn.com/libs/html5shiv/3.7.0/html5shiv.js"></script>
      <script src="//oss.maxcdn.com/libs/respond.js/1.4.2/respond.min.js"></script>
    <![endif]-->
    <style>
      body {
        padding-top: 65px;
      }

      .nobr {
        white-space: nowrap;
      }
      .reverse {
        direction: rtl;
        unicode-bidi: bidi-override;
      }

      h2.panel-title {
        font-weight: bold;
      }

      .slide {
        -webkit-transition: all .25s ease-in-out;
        -moz-transition: all .25s ease-in-out;
        -o-transition: all .25s ease-in-out;
        transition: all .25s ease-in-out;
      }
      .slide-reset {
        -webkit-transform: translateY(0);
        -ms-transform: translateY(0);
        transform: translateY(0);
      }
      .slide-up {
        -webkit-transform: translateY(-100%);
        -ms-transform: translateY(-100%);
        transform: translateY(-100%);
      }

      .factoids th.factoid-name {
        text-align: center;
      }
      .factoids td.factoid-name, .factoids td.factoid-aliases {
        vertical-align: middle;
      }
      .factoids td.factoid-aliases ul {
        margin: 0;
        padding: 0;
      }
      .factoids td.factoid-aliases li {
        list-style-type: none;
      }


      footer {
        padding-top: 40px;
        padding-bottom: 40px;
        margin-top: 40px;
        color: #777;
        text-align: center;
        border-top: 1px solid #E5E5E5;
      }
      .footer-links {
        margin-top: 20px;
        padding-left: 0;
        color: #999;
      }
      .footer-links li {
        display: inline;
        padding: 0 2px;
      }
      .footer-links li:first-child {
        padding-left: 0;
      }
    </style>
    
  </head>
  <body data-spy="scroll" data-target="#menu">
    <nav class="navbar navbar-inverse navbar-fixed-top" id="nav" role="navigation">
      <div class="container-fluid">
        <div class="navbar-header">
          <button type="button" class="navbar-toggle" data-toggle="collapse" data-target="#menu">
            <span class="sr-only">Toggle navigation</span>
            <span class="icon-bar"></span>
            <span class="icon-bar"></span>
            <span class="icon-bar"></span>
          </button>
          <a class="navbar-brand" href="#">OBScommits</a>
        </div>

        <div class="collapse navbar-collapse" id="menu">
          <ul class="nav navbar-nav">
            <li class="active"><a href="#factoids">Factoids</a></li>
            <li><a href="#command-help">Command help</a></li>
          </ul>
        </div>
      </div>
    </nav>

    <div class="container-fluid">
      <div class="row factoids">
        <div class="panel panel-default">
          <div class="panel-heading">
            <h2 id="factoids" class="panel-title">Factoids</h2>
          </div>
          <table class="table table-striped">
            <tr>
              <th class="factoid-name">Name</th>
              <th class="factoid-aliases">Aliases</th>
              <th class="factoid-text">Text</th>
            </tr>
            {{range .}}
              <tr>
                <td class="factoid-name" id="factoid-{{.Name}}">{{.Name}}</td>
                <td class="factoid-aliases">
                  {{$aliaslen := .Aliases|len}}
                  {{if gt $aliaslen 0}}
                    <ul>
                      {{range .Aliases}}
                        <li>{{.}}</li>
                      {{end}}
                    </ul>
                  {{end}}
                </td>
                <td class="factoid-text">{{.Text | linkify | ircize}}</td>
              </tr>
            {{end}}
          </table>

        </div>
      </div>
      <div class="row">
        <div class="panel panel-default">
          <div class="panel-heading">
            <h2 id="command-help" class="panel-title">Command help</h2>
          </div>
          <table class="table">
            <tr>
              <th colspan="3">Administer factoids</th>
            </tr>
            <tr>
              <td class="command-name">.add</td>
              <td class="command-arguments"><span class="nobr">&lt;factoid-trigger&gt;</span> <span class="nobr">&lt;factoid-text&gt;</span></td>
              <td class="command-description">The factoid-trigger will trigger the factoid, upon which fact the factoid-text will be printed.<br/>This command adds a new factoid.</td>
            </tr>
            <tr>
              <td class="command-name">.mod</td>
              <td class="command-arguments"><span class="nobr">&lt;factoid-trigger&gt;</span> <span class="nobr">&lt;factoid-text&gt;</span></td>
              <td class="command-description">The factoid-trigger will trigger the factoid, upon which fact the factoid-text will be printed.<br/>This command modifies an existing factoid.</td>
            </tr>
            <tr>
              <td class="command-name">.del</td>
              <td class="command-arguments"><span class="nobr">&lt;factoid-trigger&gt;</span></td>
              <td class="command-description">This command deletes an existing factoid with the given trigger and all of its aliases.</td>
            </tr>
            <tr>
              <td class="command-name">.rename</td>
              <td class="command-arguments"><span class="nobr">&lt;old-factoid-trigger&gt;</span> <span class="nobr">&lt;new-factoid-trigger&gt;</span></td>
              <td class="command-description">This command renames an existing factoid to the new trigger, the new trigger must not exist beforehand. Also updates the aliases.</td>
            </tr>

            <tr>
              <th colspan="3">Administer factoid aliases</th>
            </tr>
            <tr>
              <td class="command-name">.addalias</td>
              <td class="command-arguments"><span class="nobr">&lt;alias-trigger&gt;</span> <span class="nobr">&lt;factoid-trigger&gt;</span></td>
              <td class="command-description">The alias-trigger will trigger the factoid-trigger.<br/>This command adds a new alias.</td>
            </tr>
            <tr>
              <td class="command-name">.modalias</td>
              <td class="command-arguments"><span class="nobr">&lt;alias-trigger&gt;</span> <span class="nobr">&lt;factoid-trigger&gt;</span></td>
              <td class="command-description">The alias-trigger will trigger the factoid-trigger.<br/>This command modifies an existing alias.</td>
            </tr>
            <tr>
              <td class="command-name">.delalias</td>
              <td class="command-arguments"><span class="nobr">&lt;alias-trigger&gt;</span></td>
              <td class="command-description">This command deletes an existing alias with the given trigger.</td>
            </tr>

            <tr>
              <th colspan="3">Administer administrators</th>
            </tr>
            <tr>
              <td class="command-name">.addadmin</td>
              <td class="command-arguments"><span class="nobr">&lt;host&gt;</span></td>
              <td class="command-description">Adds an administrator identified by the given host (&quot;Jim.users.quakenet.org&quot;)<br/>Works because of QuakeNet's host-masking feature (and for static-ips of course)</td>
            </tr>
            <tr>
              <td class="command-name">.deladmin</td>
              <td class="command-arguments"><span class="nobr">&lt;host&gt;</span></td>
              <td class="command-description">Deletes an administrator with the given host</td>
            </tr>

            <tr>
              <th colspan="3">Raw irc protocol access</th>
            </tr>
            <tr>
              <td class="command-name">.raw</td>
              <td class="command-arguments"><span class="nobr">&lt;irc-protocol&gt;</span></td>
              <td class="command-description">Send everything after the command as-is to the IRC server<br/>Example: &quot;.raw PRIVMSG #obsproject :needs the colons so that space-separated things are not seen as arguments&quot;</td>
            </tr>
          </table>
        </div>
      </div>
    </div>

    <footer role="contentinfo">
      <div class="container">
        <p>Maintained by sztanpet with the help of <a href="https://github.com/sztanpet/obscommits/graphs/contributors">contributors</a>.</p>
        <p>Code licensed under the MIT license.</p>
        <ul class="footer-links muted">
          <li><a href="https://github.com/sztanpet/obscommits">GitHub Repo</a></li>
          <li>&middot;</li>
          <li><a href="https://obsproject.com">Open Broadcaster Software</a></li>
          <li>&middot;</li>
          <li><a href="http://webchat.quakenet.org/?channels=obsproject">#obsproject @ QuakeNet</a></li>
        </ul>
      </div>
    </footer>
    <script>
      (function() {
        var header = document.querySelector("#nav");
        if(window.location.hash) {
          header.classList.add("slide-up");
        }

        new Headroom(header, {
          classes: {
            initial: "slide",
            pinned: "slide-reset",
            unpinned: "slide-up"
          }
        }).init();

      }());
    </script>
  </body>
</html>