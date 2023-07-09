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

#include <dm_socket.h>

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

	if( set_reuse(serfd) == -1) {
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
	// SO_SNDTIMEO  send timeout
    // SO_RCVTIMEO  recv timeout
    
    // SO_DEBUG  
    // SO_RCVBUF
    // SO_SNDBUF    set zero to improve  

	return serfd;
}