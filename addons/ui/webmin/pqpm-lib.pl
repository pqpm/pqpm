#!/usr/bin/perl
# PQPM Webmin module library — executes as the logged-in user (SO_PEERCRED).

use strict;
use warnings;

BEGIN { push(@INC, ".."); }
use WebminCore;
&init_config();

our $pqpm_cmd = $config{'pqpm_path'} || $ENV{'PQPM_BIN'} || 'pqpm';

sub pqpm_bin {
	return $pqpm_cmd;
}

sub pqpm_run {
	my (@args) = @_;
	my $bin = &pqpm_bin();
	my $cmd = &quote_path($bin);
	foreach my $a (@args) {
		$cmd .= ' ' . &quotemeta($a);
	}
	my $out = &backquote_command($cmd . ' 2>&1');
	my $code = $?;
	$code = $code >> 8 if $code > 255;
	$out = '' unless defined $out;
	return ($code, $out);
}

sub pqpm_ping_ok {
	my ($code) = &pqpm_run('ping');
	return $code == 0;
}

sub read_user_config {
	my $home = $remote_user_info[7] || $ENV{'HOME'} || (getpwuid($<))[7];
	my $path = "$home/.pqpm.toml";
	if (!-f $path) {
		return ($path, '');
	}
	my $data = &read_file_contents($path);
	return ($path, defined $data ? $data : '');
}

sub write_user_config {
	my ($content) = @_;
	$content = '' unless defined $content;
	# Reject dangerous operators in command lines (parity with core validation).
	while ($content =~ /^\s*command\s*=\s*(.+)$/gim) {
		my $cmd = $1;
		if ($cmd =~ /;|&&|\|\||\||>|<|>|`|\$\(/) {
			return "command contains dangerous shell operator";
		}
	}
	my $home = $remote_user_info[7] || $ENV{'HOME'} || (getpwuid($<))[7];
	my $path = "$home/.pqpm.toml";
	&write_file_contents($path, $content);
	chmod(0600, $path);
	return undef;
}

1;
