FROM centos:centos7

RUN INSTALL_PKGS="openssh-server iproute net-tools" && \
    yum install -y ${INSTALL_PKGS} && \
    rpm -V ${INSTALL_PKGS} && \
    yum clean all

ADD sshd_config /etc/ssh/sshd_config

RUN mkdir /var/lib/ssh && ssh-keygen -t rsa -f /var/lib/ssh/ssh_host_rsa_key

# TODO: the .ssh/authorized_keys is supposed to be bind-mounted from a Secret.
RUN mkdir /root/.ssh && chmod 700 /root/.ssh && echo "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDscZKV33tTXHgDFsOK03VGvJMcBc8dE9SmiMcNRNZAsWxn3l5en9BEWiwpNdLHYQgXI6faKP+bwdkd7LjAc66oEX9Vg+RMzNh47eEIMbpiGUAySCCF9yqhVoB/6UqfKAyDKFNAccPyaqPBDMoge7zYfGiwkm1Mf8eCIqJ/C3k8pJXTGGd8q4SuXNfn2j9e29SvNdWNOpq3a+xFI+NMY9NP2q7HNuutov2295JOCiNwz90KqIBRWf0pJGPdJ0FPRv65lsBqFup/9sjjxZUf+aaCwbjoUzTltr3uG1RJ7sHl4oqgYFzkvqi7WP6y2X8r1kU8DiO63VIJT5HOdqbCtEM/ tnozicka@lenovo-t450s" > /root/.ssh/authorized_keys && chmod 644 /root/.ssh/authorized_keys

RUN find /var/lib/ssh -exec chgrp 0 {} \; -exec chmod g+rw {} \; -type d -exec chmod g+x {} \;

RUN chmod g+rx /root /root/.ssh && chmod 740 /root/.ssh/authorized_keys

USER 1001

ENTRYPOINT ["/usr/sbin/sshd", "-4", "-D", "-e"]
