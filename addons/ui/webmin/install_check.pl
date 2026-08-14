# install_check.pl
# Controls whether this module appears under Servers or Un-used Modules.
use strict;
use warnings;
no warnings 'redefine';
no warnings 'uninitialized';

our %config;

do 'pqpm-lib.pl';

# is_installed(mode)
# For mode 1, returns 2 if installed and usable, 1 if present, or 0 otherwise.
# For mode 0, returns 1 if installed, 0 if not.
sub is_installed
{
return ($_[0] ? 2 : 1) if -x '/usr/local/bin/pqpm' || -x '/usr/bin/pqpm';
my $bin = $config{'pqpm_path'} || 'pqpm';
if (&has_command($bin) || &has_command('pqpm')) {
	return $_[0] ? 2 : 1;
	}
return 0;
}
