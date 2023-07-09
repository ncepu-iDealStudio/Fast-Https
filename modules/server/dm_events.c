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
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789qwertyuiopasdfghjklzxcvbnm123456789123456789123456789123456789"
"</pre>";
static SSL_CTX	* g_ctx 		  = NULL;
static char 	* https_response  = "HTTP/1.1 200 OK\r\n"
									"Server: httpd\r\nContent-Length: %d\r\n"
									"Connection: keep-alive\r\n\r\n";
#endif // SERVER_DEBUG


extern void events_ssl_init()
{
    SSL_CTX *ctx ;
    SSL_library_init();
    OpenSSL_add_all_algorithms();
    SSL_load_error_strings();
    ctx = SSL_CTX_new(SSLv23_server_method());
    if (ctx == NULL) {
        ERR_print_errors_fp(stdout);
        return ;
    }
    if (SSL_CTX_use_certificate_file(ctx, "./localhost.pem" , SSL_FILETYPE_PEM) <= 0) {
        ERR_print_errors_fp(stdout);
        return ;
    }
    if (SSL_CTX_use_PrivateKey_file(ctx, "./localhost-key.pem" , SSL_FILETYPE_PEM) <= 0) {
        ERR_print_errors_fp(stdout);
        return ;
    }
    if (!SSL_CTX_check_private_key(ctx)) {
        ERR_print_errors_fp(stdout);
        return ;
    }
    g_ctx = ctx;
}


void handle_accept (lis_inf_t lis_infs, int epoll_fd ) {

	switch(lis_infs.type) {
		case HTTP:
			event_accept_http(lis_infs.fd, epoll_fd);
			break;
		case HTTPS:
			event_accept_https(lis_infs.fd, epoll_fd);
			break;
		case HTTP_PROXY:
			event_accept_http1(lis_infs.fd, epoll_fd);	
			break;
		case HTTPS_PROXY:
			event_accept_https1(lis_infs.fd, epoll_fd);	
			break;
		case TCP_PROXY:		break;
		default: break;
	}
}

void handle_read (void* data, int client_fd, int epoll_fd) {
	req_t* req = (req_t*)data;

	switch(req->type) {
		case HTTP:
			event_http_read(data, client_fd, epoll_fd);
			break;
		case HTTPS:
			handle_https_read_write(data, client_fd, epoll_fd);
			break;
		case HTTP_PROXY:
			event_http_reverse(data, client_fd, epoll_fd);
			break;
		case HTTPS_PROXY:
			event_https_reverse(data, client_fd, epoll_fd);
			break;
		case TCP_PROXY:		break;
		default: break;
	}
	
}

void handle_write (void* data, int client_fd, int epoll_fd) {
	req_t* req = (req_t*)data;

	switch(req->type) {
		case HTTP:
			event_http_write(data, client_fd, epoll_fd);
			handle_close(data, client_fd, epoll_fd);
			break;
		case HTTPS:
			break;
		case HTTP_PROXY:	break;
		case TCP_PROXY:		break;
		default: break;
	}

}

void handle_close (void* data, int client_fd, int epoll_fd) {
	req_t* req = (req_t*)data;
	// int client_fd = per_req_cli->fd;

	if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) == -1)
		perror("epoll del error");

	if( close(client_fd) == -1)
		perror("client close error");

	free(req->data);
	if(req->ssl != NULL) {
		SSL_shutdown(req->ssl);
		SSL_free(req->ssl);
	}
	free(req->ssl);
	free(req);
	
}



static void event_accept_http ( int serfd, int epoll_fd ) {
	struct sockaddr_in cliaddr;
	int socklen = sizeof(cliaddr);
	struct epoll_event ev;
	int clifd;
		// serfd is nonblocking
	while( (clifd = accept(serfd, (struct sockaddr*)&cliaddr, &socklen)) > 0) {
	
		if(set_non_blocking(clifd) == -1) {
			perror("set_non_blocking2");
			return;
		}
		
		ev.events = EPOLLIN | EPOLLET ;
		// ev.events = EPOLLIN | EPOLLET | EPOLLONESHOT;
		
		req_t* req = (req_t*)malloc(sizeof(req_t)) ;
		memset(req, 0, sizeof(req_t));
        req->fd = clifd;
		req->type = HTTP;

		ev.data.ptr = (void*)req;
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

static void event_accept_http1 ( int serfd, int epoll_fd ) {
	struct sockaddr_in cliaddr;
	int socklen = sizeof(cliaddr);
	struct epoll_event ev;
	int clifd;
		// serfd is nonblocking
	while( (clifd = accept(serfd, (struct sockaddr*)&cliaddr, &socklen)) > 0) {
	
		if(set_non_blocking(clifd) == -1) {
			perror("set_non_blocking2");
			return;
		}
		
		ev.events = EPOLLIN | EPOLLET ;
		// ev.events = EPOLLIN | EPOLLET | EPOLLONESHOT;
		
		req_t* req = (req_t*)malloc(sizeof(req_t)) ;
		memset(req, 0, sizeof(req_t));
        req->fd = clifd;
		req->type = HTTP_PROXY;

		ev.data.ptr = (void*)req;
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

static void event_accept_https ( int serfd, int epoll_fd ) {

	struct sockaddr_in addr;
	socklen_t len = sizeof(addr);
	struct epoll_event ev;
	int code;
	int retryTimes = 0;

	req_t* req = (req_t*)malloc(sizeof(req_t)) ;
	memset(req, 0, sizeof(req_t));

	req->ssl = SSL_new(g_ctx);
	req->type = HTTPS;

	if (req->ssl == NULL) {
		printf("SSL_new error.\n");
	}

	int clifd ;

	while ((clifd = accept(serfd, (struct sockaddr *)&addr, &len)) > 0) {

		printf("accept client %d [%s:%d]\n", clifd, inet_ntoa(addr.sin_addr), ntohs(addr.sin_port));

		bool isSSLAccept = true;
		
		if(set_non_blocking(clifd) == -1) {
			perror("set_non_blocking2");
			return;
		}

		SSL_set_fd(req->ssl, clifd);

		while ((code = SSL_accept(req->ssl)) <= 0 && retryTimes++ < 100) {
			if (SSL_get_error(req->ssl, code) != SSL_ERROR_WANT_READ) {
				printf("ssl accept error. %d\n", SSL_get_error(req->ssl, code));
				break;
			}
			sleep(0.1);
		}

		printf("code %d, retry times %d\n", code, retryTimes);
		if (code != 1) {
			isSSLAccept = false;
			close(clifd);
			SSL_free(req->ssl);
		}

		req->fd = clifd;
		ev.data.ptr = (void*)req;
		ev.events = EPOLLET | EPOLLIN;
		if( epoll_ctl(epoll_fd, EPOLL_CTL_ADD, clifd, &ev) == -1) {
			printf("epoll_ctl error\n");
		}
		
	}  
	if(clifd == -1) {
		if(errno != EAGAIN )
		printf("accept %s\n", strerror(errno));
		return;
	}
}

static void event_accept_https1 ( int serfd, int epoll_fd ) {

	struct sockaddr_in addr;
	socklen_t len = sizeof(addr);
	struct epoll_event ev;
	int code;
	int retryTimes = 0;

	req_t* req = (req_t*)malloc(sizeof(req_t)) ;
	memset(req, 0, sizeof(req_t));

	req->ssl = SSL_new(g_ctx);
	req->type = HTTPS_PROXY;

	if (req->ssl == NULL) {
		printf("SSL_new error.\n");
	}

	int clifd ;

	while ((clifd = accept(serfd, (struct sockaddr *)&addr, &len)) > 0) {

		printf("accept client %d [%s:%d]\n", clifd, inet_ntoa(addr.sin_addr), ntohs(addr.sin_port));

		bool isSSLAccept = true;
		
		if(set_non_blocking(clifd) == -1) {
			perror("set_non_blocking2");
			return;
		}

		SSL_set_fd(req->ssl, clifd);

		while ((code = SSL_accept(req->ssl)) <= 0 && retryTimes++ < 100) {
			if (SSL_get_error(req->ssl, code) != SSL_ERROR_WANT_READ) {
				printf("ssl accept error. %d\n", SSL_get_error(req->ssl, code));
				break;
			}
			sleep(0.1);
		}

		printf("code %d, retry times %d\n", code, retryTimes);
		if (code != 1) {
			isSSLAccept = false;
			close(clifd);
			SSL_free(req->ssl);
		}

		req->fd = clifd;
		ev.data.ptr = (void*)req;
		ev.events = EPOLLET | EPOLLIN;
		if( epoll_ctl(epoll_fd, EPOLL_CTL_ADD, clifd, &ev) == -1) {
			printf("epoll_ctl error\n");
		}
		
	}  
	if(clifd == -1) {
		if(errno != EAGAIN )
		printf("accept %s\n", strerror(errno));
		return;
	}
}



static void event_http_read(void* data, int client_fd, int epoll_fd) {
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
			handle_close(data, client_fd, epoll_fd);
			return;
		}
		total_read += bytes_read;
	}

	ev.events = EPOLLOUT | EPOLLET ;
	ev.data.ptr = data;
	if( epoll_ctl(epoll_fd, EPOLL_CTL_MOD, client_fd, &ev) == -1)
		perror("epoll_ctl error");

	// handle_write(data, client_fd, epoll_fd);
}

static void event_http_write(void* data, int client_fd, int epoll_fd) {

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
}

static void event_http_read_write(void* data, int client_fd, int epoll_fd) {
	event_http_read(data, client_fd, epoll_fd);
	event_http_write(data, client_fd, epoll_fd);
	handle_close(data, client_fd, epoll_fd);
}



static void event_https_read(void* data, int client_fd, int epoll_fd) {
	req_t* req = (req_t*)data;
	SSL* req_ssl = req->ssl;
	
	char buf[512] = {0};
	int readSize = SSL_read(req_ssl, buf, sizeof(buf));
	if (readSize <= 0) {
		printf("SSL_read error. %d\n", SSL_get_error(req_ssl, readSize));
	}
	printf("read: %d\n%s\n", readSize, buf);
}

static void event_https_write(void* data, int client_fd, int epoll_fd) {
	req_t* req = (req_t*)data;
	SSL* req_ssl = req->ssl;


	ssize_t writeSize = SSL_write(req_ssl, send_buf, sizeof(send_buf));   // 发送响应主体
	if (writeSize <= 0) {
		printf("SSL_write error. %d\n", SSL_get_error(req_ssl, writeSize));
	}

}

static void handle_https_read_write(void* data, int client_fd, int epoll_fd) {
	req_t* req = (req_t*)data;
	SSL* req_ssl = req->ssl;
	
	char buf[1024] = {0};
	int readSize = SSL_read(req_ssl, buf, sizeof(buf));
	if (readSize <= 0) {
		printf("SSL_read error. %d\n", SSL_get_error(req_ssl, readSize));
	}
	printf("read: %d\n%s\n", readSize, buf);

	char sendBuf[1024] = {0};
	int fmtSize = sprintf(sendBuf, https_response, readSize);

	printf("*********************\n%s*********************\n", sendBuf);
	int writeSize = SSL_write(req_ssl, sendBuf, strlen(sendBuf));    // 发送响应头
	printf("format size %d, write size %d\n", fmtSize, writeSize);
	if (writeSize <= 0) {
		printf("SSL_write error. %d\n", SSL_get_error(req_ssl, writeSize));
	}
	writeSize = SSL_write(req_ssl, buf, readSize);   // 发送响应主体
	if (writeSize <= 0) {
		printf("SSL_write error. %d\n", SSL_get_error(req_ssl, writeSize));
	}
	printf("format size %d, write size %d\n", fmtSize, writeSize);

	// handle_close(data, client_fd, epoll_fd);
}




// SHUT_RD close read
// SHUT_WR close write
// SHUT_RDWR both
static void handle_shutdown (int client_fd, int epoll_fd, int how) {
	if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) == -1)
		perror("epoll del error");

	if( shutdown(client_fd, how) == -1)
		perror("client shutdowm error");
}


static void event_http_reverse(void* data, int client_fd, int epoll_fd) {
	// Fast-Https receive data "A" from Client
	char buffer[4096] = {0};
	ssize_t total_read = 0;
	ssize_t bytes_read;
	while(1) {
		bytes_read = read(client_fd, buffer + total_read, 4096 - total_read);
		if(bytes_read == -1) {
			if(errno == EAGAIN || errno == EWOULDBLOCK) {
				break;
			}else{
				perror("unknow read error");
				return;
			}
		}else if(bytes_read == 0) {		// client close socket
			handle_close(data, client_fd, epoll_fd);
			return;
		}
		total_read += bytes_read;
	}
	

	// Send data "A" to Another server and get response data "B"
	int cli_sock = client_socket();
	if(cli_sock == -1) {
		printf("reverse target no response\n");
		return;
	}
	ssize_t recv_len;
	ssize_t send_len;

	send_len = send(cli_sock, buffer, total_read, 0);
	if(send_len != total_read) {
		printf("send length < to send length\n");
	}
	memset(buffer, 0, 4096);
	recv_len = recv(cli_sock, buffer, sizeof(buffer), 0);
	close(cli_sock);
	

	// Send data "B" to Client
	ssize_t n, nwrite;
	ssize_t to_data_size = recv_len;
	n = to_data_size;
	while(n > 0){
		nwrite = write(client_fd, buffer + to_data_size -n, n);
		if(nwrite < n) {
			if(nwrite == -1 && errno != EAGAIN) {
				perror("unknow write error");
				return;
			}
			break;
		}
		n -= nwrite;
	}

	handle_close(data, client_fd, epoll_fd);
}


static void event_https_reverse(void* data, int client_fd, int epoll_fd) {
	req_t* req = (req_t*)data;
	SSL* req_ssl = req->ssl;

	// Fast-Https receive data "A" from Client
	char buffer[4096] = {0};
	ssize_t total_read = 0;
	
	total_read = SSL_read(req_ssl, buffer, sizeof(buffer));
	if (total_read <= 0) {
		printf("SSL_read error. %d\n", SSL_get_error(req_ssl, total_read));
	}
	

	// Send data "A" to Another server and get response data "B"
	int cli_sock = client_socket();
	if(cli_sock == -1) {
		printf("reverse target no response\n");
		return;
	}
	ssize_t recv_len;
	ssize_t send_len;

	send_len = send(cli_sock, buffer, total_read, 0);
	if(send_len != total_read) {
		printf("send length < to send length\n");
	}
	memset(buffer, 0, 4096);
	recv_len = recv(cli_sock, buffer, sizeof(buffer), 0);
	close(cli_sock);
	

	// Send data "B" to Client
	ssize_t to_data_size = recv_len;


	ssize_t writeSize = SSL_write(req_ssl, buffer, to_data_size);   // 发送响应主体
	if (writeSize <= 0) {
		printf("SSL_write error. %d\n", SSL_get_error(req_ssl, writeSize));
	}

}
