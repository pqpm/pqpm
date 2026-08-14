#!/usr/bin/perl
# pqpm-lib.pl — common functions for the PQPM Webmin module.
# CGI scripts run as the logged-in user so pqpmd still uses SO_PEERCRED.

use strict;
use warnings;
no warnings 'redefine';
no warnings 'uninitialized';

our (%config, %text, %in);
our (@remote_user_info, $remote_user);

BEGIN { push(@INC, '..'); }
use WebminCore;
&init_config();

sub pqpm_bin
{
my $configured = $config{'pqpm_path'} || $ENV{'PQPM_BIN'} || '';
foreach my $cand ($configured, '/usr/local/bin/pqpm', '/usr/bin/pqpm', 'pqpm') {
	next if $cand eq '';
	return $cand if $cand =~ m{/} && -x $cand;
	my $found = &has_command($cand);
	return $found if $found;
	}
return $configured || '/usr/local/bin/pqpm';
}

sub pqpm_run
{
my (@args) = @_;
my $bin = &pqpm_bin();
my $cmd = &quote_path($bin);
foreach my $a (@args) {
	$cmd .= ' '.&quotemeta($a);
	}
my $out = &backquote_command($cmd.' 2>&1');
my $code = $?;
$code = $code >> 8 if $code > 255;
$out = '' unless defined $out;
return ($code, $out);
}

sub pqpm_ping_ok
{
my ($code) = &pqpm_run('ping');
return $code == 0;
}

sub user_home_dir
{
return $remote_user_info[7] if @remote_user_info && $remote_user_info[7];
return $ENV{'HOME'} if $ENV{'HOME'};
my @pw = getpwuid($<);
return $pw[7] if @pw;
return '/';
}

sub read_user_config
{
my $path = &user_home_dir().'/.pqpm.toml';
if (!-f $path) {
	return ($path, '');
	}
my $data = &read_file_contents($path);
return ($path, defined $data ? $data : '');
}

sub write_user_config
{
my ($content) = @_;
$content = '' unless defined $content;
while ($content =~ /^\s*command\s*=\s*(.+)$/gim) {
	my $cmd = $1;
	if ($cmd =~ /;|&&|\|\||\||>|<|>|`|\$\(/) {
		return 'command contains dangerous shell operator';
		}
	}
my $path = &user_home_dir().'/.pqpm.toml';
&write_file_contents($path, $content);
chmod(0600, $path);
return undef;
}

1;
