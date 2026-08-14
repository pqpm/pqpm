#!/usr/bin/perl
require './pqpm-lib.pl';

ui_print_header(undef, $text{'index_title'}, '');

if (!&pqpm_ping_ok()) {
	print &ui_alert_box(
		$text{'daemon_down'} ||
		'PQPM daemon unreachable. Install and start pqpmd, then ensure <tt>pqpm ping</tt> works for this user.',
		'warn');
} else {
	print &ui_alert_box($text{'daemon_ok'} || 'Daemon is responding.', 'success');
}

print &ui_subheading($text{'services'} || 'Services');
my ($code, $out) = &pqpm_run('status');
if ($code != 0) {
	print &ui_alert_box(&html_escape($out || 'status failed'), 'danger');
} else {
	print '<pre style="white-space:pre-wrap">', &html_escape($out || 'No services running.'), '</pre>';
}

print &ui_links_row([
	[ 'index.cgi', $text{'refresh'} || 'Refresh' ],
	[ 'config.cgi', $text{'edit_config'} || 'Edit config' ],
]);

print &ui_subheading($text{'manage'} || 'Manage a service');
print &ui_form_start('action.cgi', 'post');
print &ui_table_start(undef, undef, 2);
print &ui_table_row($text{'service_name'} || 'Service name', &ui_textbox('name', '', 30));
print &ui_table_row($text{'action'} || 'Action',
	&ui_select('action', 'restart', [
		[ 'start',   $text{'start'} || 'Start' ],
		[ 'stop',    $text{'stop'} || 'Stop' ],
		[ 'restart', $text{'restart'} || 'Restart' ],
		[ 'reload',  $text{'reload'} || 'Reload' ],
		[ 'log',     $text{'log'} || 'View log' ],
	]));
print &ui_table_end();
print &ui_form_end([[ 'ok', $text{'run'} || 'Run' ]]);

ui_print_footer('/', $text{'index'});
