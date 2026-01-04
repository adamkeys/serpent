import os

os.write(fd, f"{input}: Hello World\n".encode())
os.close(fd)