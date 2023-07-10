/* 
    *  Copyright 2023 Ajax
    *
    *  Licensed under the Apache License, Version 2.0 (the "License");
    *  you may not use this file except in compliance with the License.
    *
    *  You may obtain a copy of the License at
    *
    *    http://www.apache.org/licenses/LICENSE-2.0
    *    
    *  Unless required by applicable law or agreed to in writing, software
    *  distributed under the License is distributed on an "AS IS" BASIS,
    *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    *  See the License for the specific language governing permissions and
    *  limitations under the License. 
    *
    */
   
#include <sys/epoll.h>
#include <unistd.h>     // for close
#include <stdio.h>
#include <string.h>
#include <fcntl.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <pthread.h>
#include <stdlib.h>
#include <errno.h>
#include <stdbool.h>
#include <sys/time.h>
#define MAX_EVENT_NUM 100

int total_accept_num = 0;

static int set_non_blocking(int);
static int set_reuse(int);


static void  handle_accept(int, int);
static void  handle_read(int, int);
static void  handle_write(int, int);
static void  handle_close (int client_fd, int epoll_fd);

static int   create_socket(int);
static void* server_make();

int set_non_blocking(int fd) {

	int flag = fcntl(fd, F_GETFL, 0);
	if(flag == -1) 
		return -1;
	
	flag |= O_NONBLOCK;
	if(fcntl(fd, F_SETFL, flag) == -1)
		return -1;
	return 0;
}

int set_reuse(int i_listenfd) {
	int out = 2;
    return setsockopt(i_listenfd, SOL_SOCKET, SO_REUSEADDR, &out, sizeof(out));
}

int create_socket(int port) {

	int serfd;
	serfd = socket(AF_INET, SOCK_STREAM, 0);
	if(serfd == -1) {
		perror("socket error");
	}
	
	struct sockaddr_in addr;
	addr.sin_family = AF_INET;
	addr.sin_addr.s_addr = INADDR_ANY;
	addr.sin_port = htons(port);

	if(set_reuse(serfd) == -1) {
		perror("set_reuse error");
	}
	if( set_non_blocking(serfd) == -1){
		perror("set_non_blocking error");
	}
	if( bind(serfd, (struct sockaddr*)&addr, sizeof(addr)) == -1) {
		perror("bind error");
	}
	if( listen(serfd, 5) == -1) {
		perror("listen error");
	}
	struct timeval tv = {0, 500};
    setsockopt(serfd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(struct timeval));
	
	return serfd;
}


void handle_accept (int serfd, int epoll_fd) {
	struct sockaddr_in cliaddr;
	int socklen = sizeof(cliaddr);
	struct epoll_event ev;
	int clifd;

	while( (clifd = accept(serfd, (struct sockaddr*)&cliaddr, &socklen)) > 0) {
	
		if(set_non_blocking(clifd) == -1) {
			perror("set_non_blocking2");
			return;
		}
		
		ev.events = EPOLLIN | EPOLLET | EPOLLONESHOT;
		ev.data.fd = clifd;
		if( epoll_ctl(epoll_fd, EPOLL_CTL_ADD, clifd, &ev) == -1) {
			perror("epoll_ctl add");
			close(clifd);
		}	
	}
	if(clifd == -1) {
		if(errno != EAGAIN )
		printf("accept %s\n", strerror(errno));
		return;
	}
	// printf("%d %d\n", clifd, total_accept_num ++);
}

void handle_read (int client_fd, int epoll_fd) {
	char buf[512] = {0};
	ssize_t total_read = 0;
	ssize_t bytes_read;
	struct epoll_event ev;

	while(1) {
		bytes_read = read(client_fd, buf + total_read, 512 - total_read);
		if(bytes_read == -1) {
			if(errno == EAGAIN || errno == EWOULDBLOCK) {
				break;
			}else{
				perror("unknow read error");
				return;
			}
		}else if(bytes_read == 0) {
			handle_close(client_fd, epoll_fd);
			return;
		}
		total_read += bytes_read;
	}

	ev.events = EPOLLOUT | EPOLLET ;
	ev.data.fd = client_fd;
	epoll_ctl(epoll_fd, EPOLL_CTL_MOD, client_fd, &ev);
	// handle_write(client_fd, epoll_fd);
}

void handle_write (int client_fd, int epoll_fd) {
	ssize_t n, nwrite;
	char buf[] = "HTTP/1.1 200 OK\r\n\r\nhello"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
		     "lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj";
				 
	ssize_t data_size = strlen(buf);
	n = data_size;

	while(n > 0){
		nwrite = write(client_fd, buf + data_size -n, n);
		if(nwrite < n) {
			if(nwrite == -1 && errno != EAGAIN) {
				perror("write error");
			}
			break;
		}
		n -= nwrite;
	}
	

	handle_close(client_fd, epoll_fd);
}

void handle_close (int client_fd, int epoll_fd) {

	if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) == -1)
		perror("epoll del error");

	if( close(client_fd) == -1)
		perror("client close error");
}


void* server_make() {
	
	int serfd = create_socket(8080);
	int epoll_fd = epoll_create(100);
	
	struct epoll_event ev;
    struct epoll_event evs[ MAX_EVENT_NUM ];

	if( set_non_blocking(epoll_fd) == -1) {
		perror("epoll set non blocking ");
	}

	ev.events = EPOLLIN | EPOLLET;
	ev.data.fd = serfd;
	epoll_ctl(epoll_fd, EPOLL_CTL_ADD, serfd, &ev);

	int evnum = 0;
	int tempfd;
	struct epoll_event tempev;

	for(;;) {
		evnum = epoll_wait(epoll_fd, evs, MAX_EVENT_NUM, 10);
		// printf("%d\n", evnum);
		if(evnum == -1){
			perror("epoll wait");
			continue;
		}
			
		for(int i=0; i<evnum; i++) {
			tempfd = evs[i].data.fd;
			
			if ((evs[i].events & EPOLLHUP)||(evs[i].events & EPOLLERR)) {
				handle_close(tempfd, epoll_fd);

			} else if(tempfd == serfd) {
				handle_accept(serfd, epoll_fd);

			} else if( evs[i].events & EPOLLIN ) {
				handle_read(tempfd, epoll_fd);

			} else if( evs[i].events & EPOLLOUT ) {
				handle_write(tempfd, epoll_fd);
			} else {
				printf("unknow events\n");
			}
		}
	}
	close(serfd);
	close(epoll_fd);
}


int main() {

	server_make();

	return 0;
}
