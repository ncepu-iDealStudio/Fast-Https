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
tar -zxvf %{_sourcedir}/%{name}-%{version}.tar.gz
go env -w GO111MODULE=on
go env -w GOPROXY=https://goproxy.io,direct

%build
go build -tags=rpm .

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
install -m 0644 config/conf.d/.keep %{buildroot}/%{_datadir}/%{name}/config/conf.d/.keep
install -m 0644 config/cert/.keep %{buildroot}/%{_datadir}/%{name}/config/cert/.keep


# Installing additional files httpdoc
install -m 0644 httpdoc/root/favicon.ico %{buildroot}/%{_datadir}/%{name}/httpdoc/root/favicon.ico
install -m 0644 httpdoc/root/index.html %{buildroot}/%{_datadir}/%{name}/httpdoc/root/index.html


# Installing additional files logs
install -m 0644 logs/.keep %{buildroot}/%{_datadir}/%{name}/logs/.keep



%files
%{_bindir}/%{name}
%{_datadir}/%{name}/config/fast-https.json
%{_datadir}/%{name}/config/fastcgi.conf
%{_datadir}/%{name}/config/mime.json
%{_datadir}/%{name}/httpdoc/root/favicon.ico
%{_datadir}/%{name}/httpdoc/root/index.html
%{_datadir}/%{name}/config/conf.d/.keep
%{_datadir}/%{name}/config/cert/.keep
%{_datadir}/%{name}/logs/.keep


%doc README.md
%license LICENSE


%changelog
