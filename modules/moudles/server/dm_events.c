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

#include <dm_events.h>

#ifdef SERVER_DEBUG
static char send_buf[] = "HTTP/1.1 200 OK\r\n\r\nhello"
"<pre>"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"</pre>";
#endif // SERVER_DEBUG


void handle_accept ( int serfd, int epoll_fd ) {
	
	struct sockaddr_in cliaddr;
	int socklen = sizeof(cliaddr);
	struct epoll_event ev;
	
	// per_req_event_t* per_req_cli = (per_req_event_t*)malloc(sizeof(per_req_event_t));
	// per_req_cli->type = HTTP;
	// per_req_cli->ssl = NULL;
	// per_req_cli->data = NULL;
	int clifd;
	
	// serfd is nonblocking
	while( (clifd = accept(serfd, (struct sockaddr*)&cliaddr, &socklen)) > 0) {
	
		if(set_non_blocking(clifd) == -1) {
			perror("set_non_blocking2");
			return;
		}
		
		ev.events = EPOLLIN | EPOLLET ;
		// ev.events = EPOLLIN | EPOLLET | EPOLLONESHOT;
		// per_req_cli->fd = clifd;

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

}

void handle_read (int client_fd, int epoll_fd) {

	// int client_fd = per_req_cli->fd;

	char read_buf[512] = {0};
	ssize_t total_read = 0;
	ssize_t bytes_read;
	struct epoll_event ev;

	// serfd is nonblocking
	while(1) {
		bytes_read = read(client_fd, read_buf + total_read, 512 - total_read);
		if(bytes_read == -1) {
			if(errno == EAGAIN || errno == EWOULDBLOCK) {
				break;
			}else{
				perror("unknow read error");
				return;
			}
		}else if(bytes_read == 0) {		// client close socket
			handle_close(client_fd, epoll_fd);
			return;
		}
		total_read += bytes_read;
	}

	// ev.events = EPOLLOUT | EPOLLET ;
	// ev.data.fd = client_fd;
	// if( epoll_ctl(epoll_fd, EPOLL_CTL_MOD, client_fd, &ev) == -1)
	// 	perror("epoll_ctl error");

	handle_write(client_fd, epoll_fd);

}

void handle_write (int client_fd, int epoll_fd) {

	// int client_fd = per_req_cli->fd;

	ssize_t n, nwrite;
	ssize_t data_size = strlen(send_buf);		//events global val
	n = data_size;

	// serfd is nonblocking
	while(n > 0){
		nwrite = write(client_fd, send_buf + data_size -n, n);
		if(nwrite < n) {
			if(nwrite == -1 && errno != EAGAIN) {
				perror("unknow write error");
				return;
			}
			break;
		}
		n -= nwrite;
	}
	
	handle_close(client_fd, epoll_fd);
}

void handle_close (int client_fd, int epoll_fd) {

	// int client_fd = per_req_cli->fd;

	if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) == -1)
		perror("epoll del error");

	if( close(client_fd) == -1)
		perror("client close error");

	// free(per_req_cli->data);
	// free(per_req_cli->ssl);
	// free(per_req_cli);
}


// SHUT_RD close read
// SHUT_WR close write
// SHUT_RDWR both
void handle_shutdown (int client_fd, int epoll_fd, int how) {
	if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) == -1)
		perror("epoll del error");

	if( shutdown(client_fd, how) == -1)
		perror("client shutdowm error");
}