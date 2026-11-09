#!/usr/bin/env python3
"""Exercise the compiled editor in a real Unix pseudoterminal."""
import fcntl
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import tempfile
import termios
import time

binary = Path(__file__).resolve().parents[1] / "bin/miv"
with tempfile.TemporaryDirectory() as directory:
    path = Path(directory) / "smoke.txt"
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 80, 0, 0))
    before = termios.tcgetattr(slave)
    env = dict(os.environ, TERM="xterm-256color", WAYLAND_DISPLAY="", DISPLAY="", SHELL=os.environ.get("MIV_TEST_SHELL", "/bin/sh"))
    process = subprocess.Popen(
        [str(binary), str(path)], stdin=slave, stdout=slave, stderr=slave,
        env=env, cwd=directory, start_new_session=True,
        preexec_fn=lambda: fcntl.ioctl(0, termios.TIOCSCTTY, 0),
    )
    output = bytearray()

    def drain(seconds):
        until = time.monotonic() + seconds
        while time.monotonic() < until:
            readable, _, _ = select.select([master], [], [], 0.05)
            if readable:
                try:
                    output.extend(os.read(master, 65536))
                except OSError:
                    break

    def wait_for(marker, start, timeout=5):
        deadline = time.monotonic() + timeout
        while marker not in output[start:]:
            assert process.poll() is None, ("editor exited", output[-1500:])
            if time.monotonic() >= deadline:
                raise AssertionError(("missing output", marker, output[-1500:]))
            drain(0.05)

    def shell_command(command, reply=None, interrupt=False):
        start = len(output)
        os.write(master, (":!" + command + "\r").encode())
        if reply is not None:
            drain(0.2)
            os.write(master, (reply + "\n").encode())
        if interrupt:
            drain(0.3)
            os.write(master, b"\x03")
        wait_for(b"Press Enter to return to miv...", start)
        os.write(master, b"\r")
        drain(0.2)

    try:
        drain(0.5)
        os.write(master, "iHello, café!".encode())
        drain(0.2)
        os.write(master, b"\x1b[200~\npasted without indentation\x1b[201~")
        drain(0.2)
        os.write(master, b"\x1b")
        drain(0.15)
        shell_command("printf 'hello\\n' | tr a-z A-Z > shell-output.txt")
        assert (Path(directory) / "shell-output.txt").read_text() == "HELLO\n"
        shell_command('read answer; printf "%s" "$answer" > input.txt', reply="typed answer")
        assert (Path(directory) / "input.txt").read_text() == "typed answer"
        shell_command("exit 7")
        shell_command("sleep 10", interrupt=True)
        os.write(master, b":term\r")
        drain(0.3)
        os.write(master, b"printf interactive > term-output.txt\nexit\n")
        drain(0.4)
        assert (Path(directory) / "term-output.txt").read_text() == "interactive"
        os.write(master, b":term printf command > term-command.txt\r")
        drain(0.3)
        assert (Path(directory) / "term-command.txt").read_text() == "command"
        # Editing after returning also verifies raw mode and keyboard input recovery.
        os.write(master, b"A!")
        drain(0.1)
        os.write(master, b"\x1b")
        drain(0.15)
        # Exercise Doom leader sequences and keep a dirty buffer in another workspace.
        os.write(master, b" \t2")
        drain(0.1)
        os.write(master, b"iworkspace two")
        drain(0.1)
        os.write(master, b"\x1b")
        drain(0.15)
        os.write(master, b" \t1 fs")
        drain(0.2)
        os.write(master, b":qa\r")
        drain(0.2)
        assert process.poll() is None, "quit discarded a hidden workspace buffer"
        os.write(master, b":qa!\r")
        drain(0.4)
        process.wait(timeout=3)
        assert process.returncode == 0, (process.returncode, output[-1000:])
        assert path.read_text() == "Hello, café!\npasted without indentation!", path.read_bytes()
        assert termios.tcgetattr(slave) == before, "terminal flags were not restored"
        print("PTY smoke passed: editing, paste, shell commands, interactive shell, Ctrl-C, workspaces, leader save, terminal restoration")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        os.close(master)
        os.close(slave)
