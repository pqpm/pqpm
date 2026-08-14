#!/usr/bin/perl
use strict;
use warnings;
no warnings 'redefine';
no warnings 'uninitialized';
our (%text, %in);

require './pqpm-lib.pl';
&ReadParse();
&ui_print_header(undef, $text{'action_title'}, '');

my $name = $in{'name'} // '';
my $action = $in{'action'} // '';
$name =~ s/^\s+|\s+$//g;

if ($name eq '' || $action !~ /^(start|stop|restart|reload|log)$/) {
	print &ui_alert_box($text{'bad_input'}, 'danger');
	&ui_print_footer('index.cgi', $text{'back'});
	exit;
	}

if ($action eq 'log') {
	my ($code, $out) = &pqpm_run('log', '-n', '200', $name);
	print &ui_subheading($text{'log_for'}.': '.&html_escape($name));
	if ($code != 0) {
		print &ui_alert_box(&html_escape($out || 'log failed'), 'danger');
		}
	else {
		print '<pre style="white-space:pre-wrap">', &html_escape($out), '</pre>';
		}
	}
else {
	my ($code, $out) = &pqpm_run($action, $name);
	if ($code != 0) {
		print &ui_alert_box(&html_escape($out || "$action failed"), 'danger');
		}
	else {
		print &ui_alert_box(&html_escape($out || "$action OK"), 'success');
		}
	}

&ui_print_footer('index.cgi', $text{'back'});
