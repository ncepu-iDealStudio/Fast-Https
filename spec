Name:           fast-https
Version:        1.0.0
Release:        1%{?dist}
Summary:        fast-https web server

Group:          Development/Tools
License:        GPL
URL:            https://gitee.com/src-openeuler/fast-https
Source0:        %{name}-%{version}.tar.gz

# BuildRequires: golang 
# Requires:       

%description
fast-https web server

%prep
%setup -q -c -T
wget https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
tar xvf go1.22.4.linux-amd64.tar.gz
export PATH=$PATH:`pwd`/go/bin
tar -zxvf %{_sourcedir}/%{name}-%{version}.tar.gz
go env -w GO111MODULE=on
go env -w GOPROXY=https://goproxy.io,direct

%build
export PATH=$PATH:`pwd`/go/bin
go build -ldflags="-B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \n')" -tags=rpm .

%install
# //usr/bin
# //share/fast-https
install -d %{buildroot}/%{_bindir}
install -d %{buildroot}/%{_datadir}/%{name}/config
install -d %{buildroot}/%{_datadir}/%{name}/config/conf.d
install -d %{buildroot}/%{_datadir}/%{name}/config/cert
install -d %{buildroot}/%{_datadir}/%{name}/httpdoc/root
install -d %{buildroot}/%{_datadir}/%{name}/logs

# Installing the binary
install -m 0755 %{name} %{buildroot}/%{_bindir}/%{name}

# Installing additional files config
install -m 0644 config/fast-https.json %{buildroot}/%{_datadir}/%{name}/config/fast-https.json
install -m 0644 config/fastcgi.conf %{buildroot}/%{_datadir}/%{name}/config/fastcgi.conf
install -m 0644 config/mime.json %{buildroot}/%{_datadir}/%{name}/config/mime.json


# Installing additional files httpdoc
install -m 0644 httpdoc/root/favicon.ico %{buildroot}/%{_datadir}/%{name}/httpdoc/root/favicon.ico
install -m 0644 httpdoc/root/index.html %{buildroot}/%{_datadir}/%{name}/httpdoc/root/index.html




%files
%{_bindir}/%{name}
%{_datadir}/%{name}/config/fast-https.json
%{_datadir}/%{name}/config/fastcgi.conf
%{_datadir}/%{name}/config/mime.json
%{_datadir}/%{name}/httpdoc/root/favicon.ico
%{_datadir}/%{name}/httpdoc/root/index.html
%dir %{_datadir}/%{name}/config/conf.d
%dir %{_datadir}/%{name}/config/cert
%dir %{_datadir}/%{name}/logs


%doc README.md
%license LICENSE


%changelog
