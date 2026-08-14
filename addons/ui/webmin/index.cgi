#!/usr/bin/perl
# index.cgi — PQPM service dashboard
use strict;
use warnings;
no warnings 'redefine';
no warnings 'uninitialized';
our (%text);

require './pqpm-lib.pl';
&ui_print_header(undef, $text{'index_title'}, '', undef, 1, 1);

if (!-x &pqpm_bin()) {
	print &ui_alert_box($text{'index_nopqpm'}, 'warn');
	&ui_print_footer('/', $text{'index'});
	exit;
	}

if (!&pqpm_ping_ok()) {
	print &ui_alert_box($text{'daemon_down'}, 'warn');
	}
else {
	print &ui_alert_box($text{'daemon_ok'}, 'success');
	}

print &ui_subheading($text{'services'});
my ($code, $out) = &pqpm_run('list');
if ($code != 0) {
	($code, $out) = &pqpm_run('status');
	}
if ($code != 0) {
	print &ui_alert_box(&html_escape($out || 'status failed'), 'danger');
	}
else {
	print '<pre style="white-space:pre-wrap">', &html_escape($out || $text{'no_services'}), '</pre>';
	}

print &ui_links_row([
	[ 'index.cgi', $text{'refresh'} ],
	[ 'config.cgi', $text{'edit_config'} ],
]);

print &ui_subheading($text{'manage'});
print &ui_form_start('action.cgi', 'post');
print &ui_table_start(undef, undef, 2);
print &ui_table_row($text{'service_name'}, &ui_textbox('name', '', 30));
print &ui_table_row($text{'action'},
	&ui_select('action', 'restart', [
		[ 'start',   $text{'start'} ],
		[ 'stop',    $text{'stop'} ],
		[ 'restart', $text{'restart'} ],
		[ 'reload',  $text{'reload'} ],
		[ 'log',     $text{'log'} ],
	]));
print &ui_table_end();
print &ui_form_end([ [ undef, $text{'run'} ] ]);

&ui_print_footer('/', $text{'index'});
