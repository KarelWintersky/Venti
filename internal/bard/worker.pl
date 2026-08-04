use File::Temp;

$| = 1;

BEGIN {
    $main::VENTI_EXIT_CODE = undef;
    *CORE::GLOBAL::exit = sub { $main::VENTI_EXIT_CODE = @_ ? (0 + shift) : 0; die "__VENTI_EXIT__\n"; };
}

sub read_header {
    my $line = '';
    while (1) {
        my $n = sysread(STDIN, my $c, 1);
        return undef unless defined $n && $n == 1;
        return $line if $c eq "\n";
        $line .= $c;
    }
}

sub read_exact {
    my ($fh, $n) = @_;
    my $data = '';
    while (length($data) < $n) {
        my $r = sysread($fh, my $buf, $n - length($data));
        return undef unless defined $r && $r > 0;
        $data .= $buf;
    }
    return $data;
}

sub slurp {
    my ($fh) = @_;
    my $data = '';
    while (1) {
        my $r = sysread($fh, my $buf, 65536);
        last unless defined $r && $r > 0;
        $data .= $buf;
    }
    return $data;
}

binmode(STDIN);
binmode(STDOUT);

while (1) {
    my $line = read_header();
    last unless defined $line;
    $line =~ /^(\d+) (\d+)$/ or last;
    my ($env_len, $stdin_len) = ($1, $2);

    my $env_block = read_exact(\*STDIN, $env_len);
    my $body      = read_exact(\*STDIN, $stdin_len);
    last unless defined $env_block && defined $body;

    %ENV = ();
    for my $entry (split /\n/, $env_block) {
        my ($k, $v) = split /=/, $entry, 2;
        $ENV{$k} = defined $v ? $v : '';
    }

    my $script = $ENV{SCRIPT_FILENAME};
    if (!defined $script || !length $script || !-f $script || !-r $script) {
        print STDOUT "0 " . length("script not found: $script\n") . " 1\n";
        print STDOUT "script not found: $script\n";
        next;
    }
    $0 = $script;

    my $in = File::Temp->new();
    binmode($in);
    $in->autoflush(1);
    print {$in} $body;
    seek($in, 0, 0) or die "cannot rewind stdin temp: $!";

    open(my $saved_in,  '<&', STDIN)  or die "cannot save stdin: $!";
    open(my $saved_out, '>&', STDOUT) or die "cannot save stdout: $!";
    open(my $saved_err, '>&', STDERR) or die "cannot save stderr: $!";

    my $out = File::Temp->new();
    my $err = File::Temp->new();
    binmode($out);
    binmode($err);

    close(STDIN);
    open(STDIN, '<&', $in) or die "cannot rebind stdin: $!";
    open(STDOUT, '>&', $out)  or die "cannot rebind stdout: $!";
    open(STDERR, '>&', $err)  or die "cannot rebind stderr: $!";
    my $old_sel = select(STDOUT);
    $| = 1;
    select(STDERR);
    $| = 1;
    select($old_sel);

    my ($status, $err_msg) = (0, '');
    $main::VENTI_EXIT_CODE = undef;
    my $ret = do $script;
    my $do_err = $@;
    if (defined $main::VENTI_EXIT_CODE) {
        $status = $main::VENTI_EXIT_CODE;
    }
    elsif ($do_err) {
        $err_msg = $do_err;
        $status  = 1;
    }

    close(STDIN);
    open(STDIN, '<&', $saved_in) or die "cannot restore stdin: $!";
    open(STDOUT, '>&', $saved_out) or die "cannot restore stdout: $!";
    open(STDERR, '>&', $saved_err) or die "cannot restore stderr: $!";
    $| = 1;

    seek($out, 0, 0) or die "cannot rewind stdout temp: $!";
    seek($err, 0, 0) or die "cannot rewind stderr temp: $!";
    my $out_data = slurp($out);
    my $err_data = slurp($err);

    $err_data .= $err_msg if length $err_msg;

    print STDOUT length($out_data), ' ', length($err_data), ' ', $status, "\n";
    print STDOUT $out_data;
    print STDOUT $err_data;
}
