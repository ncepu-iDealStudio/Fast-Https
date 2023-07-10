package config

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	WorkerProcesses         int
	EventsWorkerConnections int
	HttpClientMaxBodySize   string
	HttpServer              struct {
		Listen     string
		ServerName string
		Root       string
		Index      string
		ErrorPage  string
		Location   string
	}
}

func main() {
	// Open the config file for reading
	file, err := os.Open("config.conf")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Declare a new Config struct
	config := Config{}

	// Define regular expressions to match the config file syntax
	workerProcessesRegex := regexp.MustCompile(`^worker_processes\s+(\d+)\s*;\s*$`)
	eventsWorkerConnectionsRegex := regexp.MustCompile(`^worker_connections\s+(\d+)\s*;\s*$`)
	httpClientMaxBodySizeRegex := regexp.MustCompile(`^client_max_body_size\s+(\S+)\s*;\s*$`)
	httpServerRegex := regexp.MustCompile(`^server\s*\{\s*$`)
	listenRegex := regexp.MustCompile(`^listen\s+(\S+)\s*;\s*$`)
	serverNameRegex := regexp.MustCompile(`^server_name\s+(\S+)\s*;\s*$`)
	rootRegex := regexp.MustCompile(`^root\s+(\S+)\s*;\s*$`)
	indexRegex := regexp.MustCompile(`^index\s+(\S+)\s*;\s*$`)
	errorPageRegex := regexp.MustCompile(`^error_page\s+(.*)\s*;\s*$`)
	locationRegex := regexp.MustCompile(`^location\s+(\S+)\s*\{\s*$`)

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Loop through each line of the file
	for scanner.Scan() {
		line := scanner.Text()

		// Ignore lines that start with #
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		// Check if the line matches the syntax for the worker_processes property
		if matches := workerProcessesRegex.FindStringSubmatch(line); len(matches) > 0 {
			// Store the value in the config struct
			config.WorkerProcesses, _ = strconv.Atoi(matches[1])
			continue
		}

		// Check if the line matches the syntax for the worker_connections property
		if matches := eventsWorkerConnectionsRegex.FindStringSubmatch(line); len(matches) > 0 {
			// Store the value in the config struct
			config.EventsWorkerConnections, _ = strconv.Atoi(matches[1])
			continue
		}

		// Check if the line matches the syntax for the client_max_body_size property
		if matches := httpClientMaxBodySizeRegex.FindStringSubmatch(line); len(matches) > 0 {
			// Store the value in the config struct
			config.HttpClientMaxBodySize = matches[1]
			continue
		}

		// Check if the line matches the syntax for the http server block
		if matches := httpServerRegex.FindStringSubmatch(line); len(matches) > 0 {
			// Initialize the httpServer struct
			httpServer := &config.HttpServer{}

			// Loop through the lines until the end of the server block
			for scanner.Scan() {
				line := scanner.Text()

				// Check if the line matches the syntax for the listen property
				if matches := listenRegex.FindStringSubmatch(line); len(matches) > 0 {
					// Store the value in the httpServer struct
					httpServer.Listen = matches[1]
					continue
				}

				// Check if the line matches the syntax for the server_name property
				if matches := serverNameRegex.FindStringSubmatch(line); len(matches) > 0 {
					// Store the value in the httpServer struct
					httpServer.ServerName = matches[1]
					continue
				}

				// Check if the line matches the syntax for the root property
				if matches := rootRegex.FindStringSubmatch(line); len(matches) > 0 {
					// Store the value in the httpServer struct
					httpServer.Root = matches[1]
					continue
				}

				// Check if the line matches the syntax for the index property
				if matches := indexRegex.FindStringSubmatch(line); len(matches) > 0 {
					// Store the value in the httpServer struct
					httpServer.Index = matches[1]
					continue
				}

				// Check if the line matches the syntax for the error_page property
				if matches := errorPageRegex.FindStringSubmatch(line); len(matches) > 0 {
					// Store the value in the httpServer struct
					httpServer.ErrorPage = matches[1]
					continue
				}

				// Check if the line matches the syntax for the location block
				if matches := locationRegex.FindStringSubmatch(line); len(matches) > 0 {
					// Store the value in the httpServer struct
					httpServer.Location = matches[1]

					// Loop through the lines until the end of the location block
					for scanner.Scan() {
						locationLine := scanner.Text()

						// Check if the line ends the location block
						if strings.TrimSpace(locationLine) == "}" {
							break
						}
					}

					continue
				}

				// Check if the line ends the server block
				if strings.TrimSpace(line) == "}" {
					break
				}
			}

			// Store the httpServer struct in the config struct
			config.HttpServer = *httpServer

			continue
		}
	}

	// Print out the values for testing purposes
	fmt.Println("Worker processes:", config.WorkerProcesses)
	fmt.Println("Events worker connections:", config.EventsWorkerConnections)
	fmt.Println("HTTP client max body size:", config.HttpClientMaxBodySize)
	fmt.Println("HTTP server listen:", config.HttpServer.Listen)
	fmt.Println("HTTP server name:", config.HttpServer.ServerName)
	fmt.Println("HTTP server root:", config.HttpServer.Root)
	fmt.Println("HTTP server index:", config.HttpServer.Index)
	fmt.Println("HTTP server error page:", config.HttpServer.ErrorPage)
	fmt.Println("HTTP server location:", config.HttpServer.Location)
}
