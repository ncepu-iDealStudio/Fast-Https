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

#ifndef __DM_MASTER_INCLUDE__
#define __DM_MASTER_INCLUDE__

#include <dm_server_config.h>
#include <dm_server.h>


#include <signal.h>		// signal()
#include <sys/wait.h>   // waitpid()
#include <sys/stat.h>   // umask(0)
#include <unistd.h>     // STDIN_FILENO  STDOUT_FILENO  STDERR_FILENO
#include <fcntl.h>      // O_RDWR  O_WRONLY  O_RDONLY
#include <stdbool.h>     


#ifdef __cplusplus
extern "C" {
#endif


extern void master_daemonize();
extern void master_start_multi_process_server();


static void handle_signal();
static void signal_SIGUSR1(int signum);
static void master_check_and_restart(int serfd);

#ifdef __cplusplus
}		/* end of the 'extern "C"' block */
#endif


#endif  // __DM_MASTER_INCLUDE__

