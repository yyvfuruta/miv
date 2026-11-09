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
    env = dict(os.environ, TERM="xterm-256color", WAYLAND_DISPLAY="", DISPLAY="")
    process = subprocess.Popen(
        [str(binary), str(path)], stdin=slave, stdout=slave, stderr=slave,
        env=env, start_new_session=True,
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

    try:
        drain(0.5)
        os.write(master, "iOlá, mundo!".encode())
        drain(0.2)
        os.write(master, b"\x1b[200~\ncolado sem indentacao\x1b[201~")
        drain(0.2)
        os.write(master, b"\x1b")
        drain(0.15)
        os.write(master, b":wq\r")
        drain(0.4)
        process.wait(timeout=3)
        assert process.returncode == 0, (process.returncode, output[-1000:])
        assert path.read_text() == "Olá, mundo!\ncolado sem indentacao", path.read_bytes()
        assert termios.tcgetattr(slave) == before, "terminal flags were not restored"
        print("PTY smoke passed: UTF-8, bracketed paste, :wq, terminal restoration")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        os.close(master)
        os.close(slave)
