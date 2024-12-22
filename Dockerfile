# Use an official Go runtime as a parent image
FROM golang:1.23

# Set the working directory in the container
WORKDIR /app

# Copy the current directory contents into the container at /app
COPY . /app

# Install Python and pip
RUN apt-get update && \
    apt-get install -y python3-dev

# Install Go modules
RUN go mod download

# Run tests
CMD ["go", "test"]