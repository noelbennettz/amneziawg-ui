package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"amneziawg-web-ui/internal/adminpw"
)

// programName is how the banner and the usage line spell the binary. The image
// installs it as "api", a development build is "server.bin", and the operator
// copies whichever one is on the command line in front of them.
func programName() string {
	if len(os.Args) == 0 {
		return "api"
	}
	return filepath.Base(os.Args[0])
}

// hashPassword prints the argon2id hash of the password read from r. The
// password comes from stdin and never from an argument: an argument is in the
// shell history and in the process list of every machine the command ran on.
func hashPassword(r io.Reader, w io.Writer) error {
	password, err := adminpw.ReadPassword(r)
	if err != nil {
		return err
	}
	encoded, err := adminpw.HashPassword(password)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, encoded)

	return nil
}

// resetPassword writes a new random password's hash to path and prints the
// password itself. A panel already running against that file picks it up
// without a restart, and every session it issued stops being accepted.
func resetPassword(path string, envSet bool, w io.Writer) error {
	password, err := adminpw.Reset(path)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "New admin password: %s\n", password)
	fmt.Fprintf(w, "Stored as an argon2id hash in %s (0600).\n", path)
	fmt.Fprintln(w, "A running panel adopts it within a second and ends every existing session.")
	if envSet {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Note: WEB_UI_PASSWORD is set, and an environment variable beats the stored")
		fmt.Fprintln(w, "hash. Remove it from the container's environment for the new password to")
		fmt.Fprintln(w, "take effect.")
	}

	return nil
}

// usage lists the two subcommands and what they are for.
func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [command]\n", programName())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Without a command it starts the panel and serves the web UI.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  hash-password    read a password from stdin, print its argon2id hash")
	fmt.Fprintf(w, "                   (for WEB_UI_PASSWORD; in a compose file $ is written as $$)\n")
	fmt.Fprintln(w, "  reset-password   generate a password, store its hash in /etc/amnezia/admin.hash,")
	fmt.Fprintln(w, "                   print it, and end every session the running panel issued")
	fmt.Fprintln(w, "  help             print this")
}

// announceGeneratedPassword prints the password the panel generated on its
// first start. This is the only place it is ever written down: the hash is
// what is stored, and the plaintext is gone once this process exits.
func announceGeneratedPassword(w io.Writer, user, password string) {
	line := "================================================================"
	fmt.Fprintln(w)
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, "  AmneziaWG Web UI: admin password generated")
	fmt.Fprintln(w, line)
	fmt.Fprintf(w, "  User:      %s\n", user)
	fmt.Fprintf(w, "  Password:  %s\n", password)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  This is shown once. Copy it now: only its argon2id hash is")
	fmt.Fprintln(w, "  stored, in /etc/amnezia/admin.hash, and the password cannot be")
	fmt.Fprintln(w, "  read back out of it.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  To replace it with one of your own:")
	fmt.Fprintf(w, "    docker compose exec app %s reset-password\n", programName())
	fmt.Fprintln(w, line)
	fmt.Fprintln(w)
}

// explainDefaultPassword is what a deployment that still carries the old
// default password is started with: a refusal, and the two ways out of it.
func explainDefaultPassword(w io.Writer, path string) {
	fmt.Fprintln(w, "Refusing to start: the configured password is \"changeme\".")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "The image used to ship that password, and anyone who reaches the port can")
	fmt.Fprintln(w, "type it. Pick one of these:")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  * remove WEB_UI_PASSWORD, delete %s if the volume has one, and let the\n", path)
	fmt.Fprintln(w, "    panel generate a password on the next start (it prints it once):")
	fmt.Fprintln(w, "      docker compose up -d && docker compose logs app")
	fmt.Fprintln(w, "  * or, when the stored file is what holds the old password, replace it in")
	fmt.Fprintln(w, "    place - a running panel adopts it within a second:")
	fmt.Fprintf(w, "      docker compose exec app %s reset-password\n", programName())
	fmt.Fprintln(w, "  * or set your own hash, keeping in mind that compose needs a literal $ as $$:")
	fmt.Fprintf(w, "      printf '%%s' 'your-password' | docker run --rm -i <image> %s hash-password\n", programName())
	fmt.Fprintln(w, "      # then WEB_UI_PASSWORD: \"<the hash, with $ written as $$>\" in the compose file")
	fmt.Fprintln(w)
}
